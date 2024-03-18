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
docker run -l dockerutil.autoheal=true -l dockerutil.cron=true --health-cmd "uname -a" --health-interval 10s --health-retries 3 --health-start-period 5s --health-timeout 1s -l dockerutil.cron.test2=false -l dockerutil.cron.test2.spec="2 * * * *" -l dockerutil.cron.test2.cmd="pwd" ubuntu sleep infinity
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
      - "dockerutil.autoheal=true"
```