package main

import (
	"bufio"
	"context"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/go-co-op/gocron/v2"
	"github.com/rs/zerolog/log"
)

// CronManager manages cron job tasks
type CronManager struct {
	cli       *client.Client
	scheduler *gocron.Scheduler
	queue     chan cronJobTask
}

// cronJobTask represents a cron job task
type cronJobTask struct {
	containerID string
	cmd         string
}

// NewCronManager creates a new instance of CronManager
func NewCronManager(cli *client.Client, scheduler *gocron.Scheduler, queueSize int) *CronManager {
	return &CronManager{
		cli:       cli,
		scheduler: scheduler,
		queue:     make(chan cronJobTask, queueSize),
	}
}

// Enqueue adds a new cron job task to the queue
func (cm *CronManager) Enqueue(task cronJobTask) {
	cm.queue <- task
}

// Dequeue removes and returns a cron job task from the queue
func (cm *CronManager) Dequeue() cronJobTask {
	return <-cm.queue
}

// HandleQueue handles tasks from the queue
func (cm *CronManager) HandleQueue(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case task := <-cm.queue:
			cm.execCronJob(ctx, task)
		}
	}
}

func main() {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to Docker daemon")
	}
	defer cli.Close()

	scheduler, err := gocron.NewScheduler()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize the scheduler")
	}
	scheduler.Start()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cronManager := NewCronManager(cli, &scheduler, 100)
	go cronManager.HandleQueue(ctx)

	f := filters.NewArgs()
	f.Add("type", "container")
	options := types.EventsOptions{
		Filters: f,
	}
	eventChan, errs := cli.Events(context.Background(), options)
	defer func() {
		if errs != nil {
			log.Fatal()
		}
	}()

	log.Info().Msg("Dockerutil started")

	if err := listContainersAndCreateCronJobs(ctx, cronManager); err != nil {
		log.Fatal().Err(err).Msg("Failed to list containers and create cron jobs")
	}

	for event := range eventChan {
		handleEvent(ctx, event, cronManager)
	}
}

func listContainersAndCreateCronJobs(ctx context.Context, cm *CronManager) error {
	containers, err := cm.cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return err
	}
	for _, container := range containers {
		createCronJobs(ctx, container.ID, cm)
	}
	return nil
}

func createCronJobs(ctx context.Context, containerID string, cm *CronManager) {
	inspect, err := cm.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		log.Error().Err(err).Str("containerID", containerID).Msg("Failed to inspect container")
		return
	}

	for label := range inspect.Config.Labels {
		if strings.HasPrefix(label, "dockerutil.cron.") {
			parts := strings.Split(label, ".")
			if len(parts) != 3 {
				continue
			}
			taskName := parts[2]
			cronSpec := inspect.Config.Labels["dockerutil.cron."+taskName+".spec"]
			cronCmd := inspect.Config.Labels["dockerutil.cron."+taskName+".cmd"]

			job, err := (*cm.scheduler).NewJob(gocron.CronJob(cronSpec, false), gocron.NewTask(func() {
				cm.Enqueue(cronJobTask{
					containerID: containerID,
					cmd:         cronCmd,
				})
			}), gocron.WithTags(containerID), gocron.WithName(taskName))

			if err != nil {
				log.Error().Err(err).Str("containerID", containerID).Str("taskName", taskName).Msg("Failed to add cron job")
				return
			} else {
				log.Info().Str("containerID", containerID).Str("taskName", taskName).Msg("Added cron job")
				if inspect.Config.Labels["dockerutil.cron."+taskName] == "true" {
					_ = job.RunNow()
				}
			}
		}
	}
}

func handleEvent(ctx context.Context, event events.Message, cm *CronManager) {
	if event.Type != events.ContainerEventType {
		return
	}

	containerID := event.Actor.ID

	switch event.Action {
	case "start":
		log.Info().Str("containerID", containerID).Msg("Container started")
		createCronJobs(ctx, containerID, cm)
	case "die", "stop":
		log.Info().Str("containerID", containerID).Msg("Container stopped")
		removeCronJobByTag(cm, containerID)
	case "health_status":
		if event.Actor.Attributes["health_status"] == "unhealthy" {
			log.Info().Str("containerID", containerID).Msg("Container is unhealthy, restarting")
			err := restartUnhealthyContainer(cm.cli, containerID)
			if err != nil {
				log.Err(err).Str("containerID", containerID).Msg("Failed to restart container")
			}
		}
	}
}

func restartUnhealthyContainer(cli *client.Client, containerID string) error {
	inspect, err := cli.ContainerInspect(context.Background(), containerID)
	if err != nil {
		return err
	}

	if inspect.Config.Labels["dockerutil.autoheal"] == "true" {
		if err := cli.ContainerRestart(context.Background(), containerID, container.StopOptions{}); err != nil {
			return err
		}
		log.Info().Str("containerID", containerID).Msg("Container restarted successfully")
	}
	return nil
}

func (cm *CronManager) execCronJob(ctx context.Context, task cronJobTask) {
	exec, err := cm.cli.ContainerExecCreate(ctx, task.containerID, types.ExecConfig{AttachStdout: true, AttachStderr: true, Cmd: strings.Split(task.cmd, " ")})
	if err != nil {
		log.Error().Err(err).Str("containerID", task.containerID).Str("cmd", task.cmd).Msg("Failed to execute cron job")
		removeCronJobByTag(cm, task.containerID)
		return
	}

	res, err := cm.cli.ContainerExecAttach(ctx, exec.ID, types.ExecStartCheck{})
	if err != nil {
		log.Error().Err(err).Str("containerID", task.containerID).Str("cmd", task.cmd).Msg("Failed attaching to container for execute cron job output")
		return
	}

	log.Info().Str("containerID", task.containerID).Str("cmd", task.cmd).Msg("Cron job command output for container:")
	defer res.Close()

	reader := bufio.NewScanner(res.Reader)
	reader.Split(bufio.ScanLines)
	for reader.Scan() {
		log.Info().Msg(reader.Text())
	}
}

func removeCronJobByTag(cm *CronManager, containerID string) {
	(*cm.scheduler).RemoveByTags(containerID)
	log.Info().Str("containerID", containerID).Msg("All cron jobs removed")
}
