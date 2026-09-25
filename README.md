# gitlab-kanboard-gateway

This is a simple tool that runs in the background, receives webhook push event requests from gitlab, and creates
kanboard ticket comments from them.

In order to use this program, you should aready have:

* A running gitlab instance: https://about.gitlab.com/
* A running kanboard instance: https://kanboard.org/

## Installation

### Prepare your gitlab-kanboard-gateway server

* Create/use a regular user (don't run it as root)
* Select an IP address and a port for your service

### Prepare your kanboard instance

* Create a normal user in your kanboard, take its user id. This technical user will be adding comments to kanboard tasks.
* Add this user to some or all of your kanboard projects. If you don't assign the user to a project,
  then no comments can be added to the tasks of that project.
* Get your API token and api endpoint in kanboard (under global settings e.g. https://your_kanboard.com/settings/api)

### Prepare your gitlab instance

* Open a project in gitlab, go to settings/webhooks
* Add a webhook for "push" events. Don't forget to set a secret token (X-Gitlab-Token header)
* It might be necessary to add your server's address under "admin area / settings / network / outbound requests"

### Create config file

Create a config file for gitlab-kanboard-gateway. You can get an example config file by executing:

```bash
gitlab-kanboard-gateway --info
```

### Start, test, troubleshoot

First, start in verbose:

```bash
gitlab-kanboard-gateway -v -c config.yml
```

Then check if it can connect to KanBoard and load your projects and tasks. Then go to your webhook in gitlab, and send
a test push event. Also, try to push a commit with kanboard task reference(s) and check if it works.

For troubleshooting, start with `--debug`.

If it works, then you can add a system service unit (Linux). Under Windows, I recommend using NSSM (https://nssm.cc/).

## kanboard-task: read-only task dump for AI agents

The repository also builds a second binary, `kanboard-task`. It looks up one or more Kanboard tasks and prints
everything about them as JSON on stdout: description, resolved project / column / swimlane / category, owner and
creator, tags, subtasks, comments, attachments, internal links to other tasks, and external links. It never writes
to Kanboard, so it is safe to hand to an AI agent that needs to read tickets.

It uses the same config file as the gateway, but only the `Kanboard` section (`ApiUrl`, `Username`, `Password`)
is read, so a minimal config with just those three keys also works.

```bash
kanboard-task -c config.yml 13670                       # one task -> JSON object
kanboard-task -c config.yml 13670 13671                 # several -> JSON array
kanboard-task -c config.yml '#KB13670'                  # accepts #KB refs and task URLs too
kanboard-task -c config.yml -o ./attachments 13670      # also download attachments to ./attachments/13670/
kanboard-task --info                                    # describes every output field
```

Logs go to stderr, only JSON goes to stdout. Exit code is 0 on success, 1 for usage/config errors,
2 when a task does not exist and 3 on Kanboard API errors. The config path can also be given in the
`KANBOARD_TASK_CONFIG` environment variable.
