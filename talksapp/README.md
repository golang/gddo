talksapp
========

This directory contains the source for [talks.godoc.org](https://talks.godoc.org).

Development Environment Setup
-----------------------------

- Copy `app.yaml` to `prod.yaml` and put in the authentication data.
- Install Go App Engine SDK.
- `$ sh setup.sh`
- Run the server using the `goapp serve prod.yaml` command.
- Run the tests using the `goapp test` command.
