# Reload on Github Push

After following this guide, the Honeydipper daemon should not pull from config repo as often as before, and it can reload when there is any change to the remote repo.

<!-- toc -->

- [Github Integration in Honeydipper](#github-integration-in-honeydipper)
- [Config webhook in Github repo](#config-webhook-in-github-repo)
- [Configure a reloading rule](#configure-a-reloading-rule)
- [Reduce the polling interval](#reduce-the-polling-interval)

<!-- tocstop -->

## Github Integration in Honeydipper

Create a yaml file in your config repo to store the settings for github integration, and make sure it is loaded through `includes` in `init.yaml`. See the [github integration reference](https://honeydipper.github.io/honeydipper-config-essentials/#HoneydipperConfigClass:Essentials.Systems.github) for detail on how to config. 

For example:

```yaml
# integrations.yaml
---
systems:
  ...
  github:
    token: ENC[gcloud-kms,xxxxxx..]
    oauth_token: ENC[gcloud-kms,xxxxxx...]
```

By configuring the github integration, we enabled a webhook at certain url (by default, `/github/push`, see your infrastructure configuration for the url host and port). As of now, the Honeydipper webhook driver doesn't support authentication using signature header, so we use a token to authenticate requests coming from github.

## Config webhook in Github repo

Go to your config repo in github, click `settings` => `webhooks`, then add a `webhook` with the webhook url.  For example,

```
https://mywebhook.example.com:8443/github/push?token=xxxxxxxx
``` 

Make sure you select "Pushes" to be sent to the configured webhook.

## Configure a reloading rule

Create a yaml file in your config repo to store a rule, and make sure it is loaded through `includes` in one of previously loaded yaml file. The rule should look like below

```yaml
# reload_on_gitpush.yaml
---
rules:
  - when:
      source:
        system: github
        trigger: push
    do:
      type: if
      condition: '{{ and (eq .event.json.repository.name "honeydipper-config") (eq .event.json.ref "refs/heads/master") }}'
      content:
        - content: reload
```
Your repository name and branch name may differ.

After the rule is loaded into the Honeydipper daemon, you should be able to see from the logs that the daemon reloads configuration when there is new push to your repo.

## Reduce the polling interval

In the configuration for your daemon, set the `configCheckInterval` to a longer duration. The duration is parsed using [ParseDuration](https://golang.org/pkg/time/#ParseDuration) API,  use 'm' suffix for minutes, 'h' for hours. See below for example:

```yaml
# daemon.yaml
---
drivers:
  daemon:
    configCheckInterval: "60m"
```
