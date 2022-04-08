

# resourcer
Single machine resource manager

resourcer is a command line utility to manage CPU/Memory resource requests on a
single machine. It works as a shim, pausing until resources are available and
then executing the command. resourcer does not actually track RAM or CPU usage,
rather the user declares how much will be needed.  Resourcer waits until that number
is avalible given the apps it is managing.

## Why is this needed?
The first use case was the Cromwell workflow engine. It's local execution mode
does not track CPU/Memory usage. Resource tracking is only enabled on other
backends. In local mode, Cromwell immediately executes all programs that have
all of their inputs available. If there are 24 tasks that each require 30GB of
RAM, Cromwell will try to run all of them at the same time. By using resourcer
as a shim executor, you can pause execution of programs until the system is less
overloaded.

## Setup
By default, resourcer will allow the programs it manage to allocate the total number
of cores in a system and 90% of the RAM.

To initialize another configuration, use the `init` function.

To set the resource limit to be 25GB of memory and 16 cores:
```
resourcer init -m 25GB -c 16
```

Then to run a program, requesting 3 cores and 8GB of RAM:
```
resourcer run -n 3 -m 8GB -- /my/cool/script.sh -a 9
```

resourcer will pause if there are too many other pending requests


## Example cromwell config

```
include required(classpath("application"))

webservice {
  port = 9000
  interface = localhost
}

backend {
  default = default

  providers {
    default {
      # The backend custom configuration.
      actor-factory = "cromwell.backend.impl.sfs.config.ConfigBackendLifecycleActorFactory"

      config {
      	run-in-background = true
      	runtime-attributes = """
      	  String? docker
      	  String? docker_user
          Int? memory_gb = 1GB
          Int? cpu = 1
      	"""

      	submit = "resourcer run -m ${memory_gb}GB -n ${cpu} -- ${job_shell} ${script}"

      	submit-docker = """
          resourcer run -m ${memory_gb}GB -n ${cpu} -- \
        	docker run \
        	 --rm -i \
        	 ${"--user " + docker_user} \
        	 --entrypoint ${job_shell} \
        	 -v ${cwd}:${docker_cwd} \
        	 ${docker} ${docker_script}
         """
      }
    }
  }
}

```
