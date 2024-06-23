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
* Add this user to some or all ofr your kanboard projects. If you don't assign the user to a project,
  then no comments can be added to the tasks of that project.
* Get you API token and api endpoint in kanboard (under global settings e.g. https://your_kanboard.com/settings/api)

### Prepare your gitlab instance

* Open a projekt in gitlab, go to settings/webhooks
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

