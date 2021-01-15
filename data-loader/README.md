This Data Loader supports preparation of a newly created (or updated) cluster by creating "standard" entities via the Admin API. For example, Salus will accumulate a growing catalog of supported telegraf agent releases where that catalog can be managed by this tool.

## Authentication

The following command-line options are used to configure the authentication of the Admin API operations used by the data loader. The username and password/apikey must correspond to a user that has appropriate admin access. Either password or apikey may be provided.

-  `--identity-url`, default is `https://identity.api.rackspacecloud.com`
-  `--identity-username` 
-  `--identity-password` 
-  `--identity-apikey`

**NOTE** when the admin URL is configured with a "localhost", authentication will be disabled and none of the configuration described above is required.

## Source content

The data loader needs to be told what content to pre-load or incrementally load into a system and two types of sources are currently supported.

### From git

In production, loader content needs to be source controlled, so the `--from-git-repo` options enables that mechanism. By default, the latest commit on the default branch (usually `master`) will be used; however, a specific commit SHA can be specified. _This latter option exercises the logic that will be utilizes when Github webhook support is implemented._ 

When cloning from a private Github repo, an access token needs to be provided via the command-line or the environment variable `GITHUB_TOKEN`. When creating the token, only the `repo` scope needs to be enabled.

-  `--from-git-repo`
-  `--from-git-sha`
-  `--github-token`

### From local directory

Primarily for development, the loader content can also use an existing directory. The [testdata](testdata) directory in this repository is ready to be used as such.

-  `--from-local-dir`

## Debugging the Webhook Server option

The data loader is primarily intended to run as a webhook server to process Github push notifications. It is currently deployed in each Salus cluster, but for development and debugging purposes it is ideal to run the data loader locally in IntelliJ and process webhook operations with that a local instance of the Salus Admin API.

The following provides the major steps needed:

- [Obtain a Github access token](https://github.com/settings/tokens/new) since that will need to be configured with the data loader to access the private [salus-data-loader-content repository](https://github.com/Rackspace-Segment-Support/salus-data-loader-content). Only the "repo" scope needs to be selected.
- Build and start the data-loader webhook server by executing it with the following `./data-loader --debug --admin-url http://localhost:8888 webhook-server` assuming `GITHUB_TOKEN` has been exported as an environment variable
- [Install and run ngrok](https://docs.github.com/en/free-pro-team@latest/developers/webhooks-and-events/configuring-your-server-to-receive-payloads) to proxy to your data loader instance, running on port 8080 by default. For example, `ngrok http 8080`
- [TEMPORARILY declare a webhook in the data loader content repo](https://github.com/Rackspace-Segment-Support/salus-data-loader-content/settings/hooks)
  - Github documentation [is available here](https://docs.github.com/en/free-pro-team@latest/developers/webhooks-and-events/creating-webhooks)
  - For "Payload URL" copy the ngrok provided HTTPS URL, append the path `/webhook` to that
  - Set "Content type" to "application/json"
  - Leave "Which events..." to "Just the push event"
- In the "Recent Deliveries" section of the temporary web hook you should see a "ping" delivery that was successful
  - In the data loader debug logs you should also see a message like `ignoring unsupported webhook event type {"type": "*github.PingEvent"}`
- Clone the salus-data-loader-content repo, create a local branch, and push some "testing changes" such as whitespace additions to the `README.md` to trigger webhooks that can be executed by data-loader

> NOTE: When finished, be sure to stop the ngrok process and delete the webhook declared in the repository settings.
    