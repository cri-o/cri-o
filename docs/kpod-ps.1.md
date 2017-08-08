% kpod(1) kpod-ps - Simple tool to list containers
% Urvashi Mohnani
% kpod-ps "1" "AUGUST 2017" "kpod"

## NAME
kpod-ps - Prints out information about containers

## SYNOPSIS
**kpod ps [OPTIONS] CONTAINER**

## DESCRIPTION
**kpod ps** lists the running containers on the system. Use the **--all** flag to view
all the containers information.  By default it lists:

 * container id
 * the name of the image the container is using
 * the COMMAND the container is executing
 * the time the container was created
 * the status of the container
 * port mappings the container is using
 * alternative names for the container

**kpod [GLOBAL OPTIONS]**

**kpod [GLOBAL OPTIONS] ps [OPTIONS]**

## GLOBAL OPTIONS

**--help, -h**
  Print usage statement

## OPTIONS

**--all, -a**
    Show all the containers, default is only running containers

**--no-trunc**
    Display the extended information

**--quiet, -q**
    Print the numeric IDs of the containers only

**--format**
    Pretty-print containers to JSON or using a Go template

Valid placeholders for the Go template are listed below:

| **Placeholder** | **Description**                                  |
| --------------- | ------------------------------------------------ |
| .ID             | Container ID                                     |
| .Image          | Image ID/Name                                    |
| .Command        | Quoted command used                              |
| .CreatedAt      | Creation time for container                      |
| .RunningFor     | Time elapsed since container was started         |
| .Status         | Status of container                              |
| .Ports          | Exposed ports                                    |
| .Size           | Size of container                                |
| .Names          | Name of container                                |
| .Labels         | All the labels assigned to the container         |
| .Label          | Value of the specific label provided by the user |
| .Mounts         | Volumes mounted in the container                 |


**--size, -s**
    Display the total file size

**--last, -n**
    Print the n last created containers (all states)

**--latest, -l**
    show the latest container created (all states)

**--filter, -f**
    Filter output based on conditions given

Valid filters are listed below:

| **Filter**      | **Description**                                                     |
| --------------- | ------------------------------------------------------------------- |
| id              | [ID] Container's ID                                                 |
| name            | [Name] Container's name                                             |
| label           | [Key] or [Key=Value] Label assigned to a container                  |
| exited          | [Int] Container's exit code                                         |
| status          | [Status] Container's status, e.g *running*, *stopped*               |
| ancestor        | [ImageName] Image or descendant used to create container            |
| before          | [ID] or [Name] Containers created before this container             |
| since           | [ID] or [Name] Containers created since this container              |
| volume          | [VolumeName] or [MountpointDestination] Volume mounted in container |

## COMMANDS

```
sudo kpod ps -a
CONTAINER ID   IMAGE         COMMAND         CREATED       STATUS                    PORTS     NAMES
02f65160e14ca  redis:alpine  "redis-server"  19 hours ago  Exited (-1) 19 hours ago  6379/tcp  k8s_podsandbox1-redis_podsandbox1_redhat.test.crio_redhat-test-crio_0
69ed779d8ef9f  redis:alpine  "redis-server"  25 hours ago  Created                   6379/tcp  k8s_container1_podsandbox1_redhat.test.crio_redhat-test-crio_1
```

```
sudo kpod ps -a -s
CONTAINER ID   IMAGE         COMMAND         CREATED       STATUS                    PORTS     NAMES                                                                  SIZE
02f65160e14ca  redis:alpine  "redis-server"  20 hours ago  Exited (-1) 20 hours ago  6379/tcp  k8s_podsandbox1-redis_podsandbox1_redhat.test.crio_redhat-test-crio_0  27.49 MB
69ed779d8ef9f  redis:alpine  "redis-server"  25 hours ago  Created                   6379/tcp  k8s_container1_podsandbox1_redhat.test.crio_redhat-test-crio_1         27.49 MB
```

```
sudo kpod ps -a --format "{{.ID}}  {{.Image}}  {{.Labels}}  {{.Mounts}}"
02f65160e14ca  redis:alpine  tier=backend  proc,tmpfs,devpts,shm,mqueue,sysfs,cgroup,/var/run/,/var/run/
69ed779d8ef9f  redis:alpine  batch=no,type=small  proc,tmpfs,devpts,shm,mqueue,sysfs,cgroup,/var/run/,/var/run/
```

## ps
Print a list of containers

## SEE ALSO
kpod(1), crio(8), crio.conf(5)

## HISTORY
August 2017, Originally compiled by Urvashi Mohnani <umohnani@redhat.com>
