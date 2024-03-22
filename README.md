# dockerutil

A simple go app over `docker` to do all tasks, which should be included with docker already...

## Features
- Easy to implement autoheal and crontab for you containers.
- Allows easy configuration using only labels.
- `dockerutil.autoheal` & `dockerutil.cron`.
- Lightweight.

## Example Config

### docker
```
docker run -l dockerutil.autoheal=true --health-cmd "uname -a" --health-interval 10s --health-retries 3 --health-start-period 5s --health-timeout 1s -l dockerutil.cron.test1=true -l dockerutil.cron.test1.spec="* * * * *" -l dockerutil.cron.test1.cmd="ls -al" -l dockerutil.cron.test2=false -l dockerutil.cron.test2.spec="2 * * * *" -l dockerutil.cron.test2.cmd="pwd" ubuntu sleep infinity
```

### compose
```
tomcat:
    container_name: tomcat
    restart: always
    image: tomcat
    volumes:
      - tomcat:/usr/local/tomcat
    healthcheck:
      test: ["CMD-SHELL", "/usr/bin/wget --spider http://localhost:8080/"]
      interval: 120s
      timeout: 5s
      retries: 3
    labels:
      - dockerutil.autoheal=true //restarts the container if health status is unhealthy
      - dockerutil.cron.getcontent=true //enables one time execution of getcontent when container starts - false use only schdule given in the spec label
      - dockerutil.cron.getcontent.spec="12 * * * *" //starts the cron cmd evey hour 12th minute
      - dockerutil.cron.getcontent.cmd="/usr/bin/wget" //command that container should execute
      - dockerutil.cron.updatecontent=false //second cron named updatecontent do not start the cronjob on container start, only run at specified schedule
      - dockerutil.cron.updatecontent.spec="* * * * *"
      - dockerutil.cron.updatecontent.cmd="/usr/bin/curl"
```