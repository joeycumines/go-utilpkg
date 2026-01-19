---
trigger: always_on
---

This project's core code quality checks are orchestrated using GNU Make, on all platforms, including in CI.

Local and project overrides and custom targets are supported via special override files, config.mk and project.mk, respectively. The former is in the gitignore. See also example.config.mk, if config.mk doesn't already exist.

ALL checks must pass *reliably*, on all platforms. It is never acceptable to have failing checks - always prioritise properly fixing failing checks (even if they only fail intermittently) as soon as possible.

There is an example of how to run all checks in a docker container in example.config.mk - e.g. to easily test Linux, from macOS.
