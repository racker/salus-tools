# Salus End to End Test
The end to end test, e2et, is meant to excercise every microservice in the system and return an inication of where failures are occuring.  It creating a local private poller, and the monitors and tasks to confirm that everything is working.  More details here: [e2et design doc](https://github.com/racker/salus-docs/blob/master/design/end-to-end-test/design.md)

## Build/Invocation
Running *make* creates the e2et exe.  The exe is run with a config file, like so:
```
./e2et   -config=config.yml
```

## Configuration
There are two basic modes in which the tests run, local and full, (depending on the corresponding value in the config file).  The local mode is a slightly stripped down version which requires less configuration, mostly of ssl certs etc, (you should just be able to run included config-local.yml file without changes.)  It also doesn't use a public poller so you don't need to set that up separately.  It also doesn't require the auth service.  So if you start the other services locally, and run the commands:
```
make
./e2et   -config=config-local.yml
```
It should all work.

The full mode excercises everything including the auth service, and public pollers and requires significantly more configuration.  It requires a regular user account as well as an admin account.  A sample full config file is [here](file://./config-dev.yml).  In addition, it requires the following env vars to be set:
```
E2ET_REGULAR_API_KEY   the api key of regular user account
E2ET_ADMIN_API_KEY     the api key of admin user account
E2ET_ADMIN_PASSWORD    the password of admin user account
```
Only one of the two ADMIN env vars is required.

## Curl commands
The test doesn't use curl, but as it invokes each api command it prints out the analogous curl, to allow invocation by hand if that seems useful.  The tokens are not printed out by default, but you can print them by adding this env var:
```
export E2ET_PRINT_TOKENS=true
```
