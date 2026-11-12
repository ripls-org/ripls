#!/usr/bin/dumb-init /bin/bash
# shellcheck shell=bash
# (dumb-init execs /bin/bash; shellcheck can't infer that from the shebang.)
# Wraps the upstream myoung34/github-runner entrypoint to scrub config-state
# files that the upstream script leaves behind. With them in place, config.sh
# refuses to (re)register on container restart and exits with
# 'already configured' / 'Value cannot be null. (Parameter configuredSettings)',
# putting the container into a crash loop.
#
# Files removed:
#   .runner                  config JSON (upstream also removes this; defensive)
#   .credentials             encrypted session creds, regenerated per registration
#   .credentials_rsaparams   RSA params, regenerated per registration
#   .runner_migrated         V2-broker migration marker written by Runner.Listener
#                            when GitHub flips a runner to the V2 architecture
#                            (broker.actions.githubusercontent.com). config.sh
#                            treats its presence as 'already configured' and
#                            refuses to register, even though .runner is gone.
rm -f /actions-runner/.runner \
      /actions-runner/.credentials \
      /actions-runner/.credentials_rsaparams \
      /actions-runner/.runner_migrated
exec /entrypoint.upstream.sh "$@"
