This Data Loader supports preparation of a newly created (or updated) cluster by creating "standard" entities via the Admin API. For example, Salus will accumulate a growing catalog of supported telegraf agent releases where that catalog can be managed by this tool.

## Authentication

The following command-line options are used to configure the authentication of the Admin API operations used by the data loader. The username and password/apikey must correspond to a user that has appropriate admin access. Either password or apikey may be provided.

-  `--identity-url`, default is `https://identity.api.rackspacecloud.com`
-  `--identity-username` 
-  `--identity-password` 
-  `--identity-apikey`

**NOTE** when the admin URL is configured with a "localhost", authentication will be disabled and none of the configuration described above it required.

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
