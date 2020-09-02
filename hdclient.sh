#!/bin/bash


#######################################################
# 
# Copyright 2019 Honey Science Corporation
# 
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, you can obtain one at http://mozilla.org/MPL/2.0/.
# 
#######################################################


#######################################################
# 
# This script includes some functions that can be used
# for interacting with Honeydipper through webhook or
# APIs. It can be used interactively or as a dependency
# to another script.
#
# The best way to make the functions available is to
# source in this file from your shell's rc file.
#
# To easily configure the access to your Honeydipper
# daemon, place a file named honeydipper in your home
# directory under .config, should include following
# environment variables
# 
# export DEFAULT_WEBHOOK_URLPREFIX="< webhook prefix e.g. https://dipper-webhook.myhoneydipper.com >"
# export DEFAULT_API_URLPREFIX="< api prefix e.g. https://dipper-api.myhoneydipper.com >"
# export HD_API_TOKEN="< api token >"
# export HD_WEBHOOK_TOKEN="< webhook token >"
#
# examples:
#
# $ hdget events
#
#     this will list all the events currently executing
#     workflows in json format.
#
# $ hdwebhook mywebhook/test
#
#     this will send a webhook request with required
#     such as https://dipper-webhook.myhoneydipper.com/mywebhook/test
#     The eventID will be stored in environment variable
#     HD_EVENT_ID.
#
# $ hdwwait
#
#     this will wait for the event $HD_EVENT_ID to finish
#     executing the workflows. check the results with
#     $HD_SESSION_SUCCESS and $HD_SESSION_FAILURE_ERROR
#
#######################################################


if [[ -f ~/.config/honeydipper ]]; then
  source ~/.config/honeydipper
fi

function hdget() {
  local api="$1"
  local urlprefix="${2-$DEFAULT_API_URLPREFIX}"

  if [[ -z "$api" ]]; then
    echo api not specified >&2
    return 1
  fi

  if [[ -z "$urlprefix" ]]; then
    echo api urlprefix not specified >&2
    return 1
  fi

  if [[ -z "$HD_API_TOKEN" ]]; then
    echo HD_API_TOKEN not specified >&2
    return 1
  fi

  # reuse HD_RETURN unless not defined
  if [[ -z "$HD_RETURN" ]]; then
    export HD_RETURN="$(mktemp)"
  fi

  export HD_STATUS_CODE="$(curl -s -o "$HD_RETURN" -w "%{http_code}" -H "Authorization: bearer $HD_API_TOKEN" "$urlprefix/$api")"
  local ret="$?"

  if [[ -z "$HD_SILENT" ]]; then
    cat "$HD_RETURN"; echo # add a new line
  fi

  return "$ret"
}

function hdwebhook() {
  local hook="$1"
  local urlprefix="${2-$DEFAULT_WEBHOOK_URLPREFIX}"

  if [[ -z "$hook" ]]; then
    echo hook not specified >&2
    return 1
  fi

  if [[ -z "$HD_WEBHOOK_TOKEN" ]]; then
    echo webhook urlprefix not specified >&2
    return 1
  fi

  if [[ -z "$HD_WEBHOOK_TOKEN" ]]; then
    echo HD_WEBHOOK_TOKEN not specified >&2
    return 1
  fi

  # reuse HD_WEBHOOK_RETURN unless not defined
  if [[ -z "$HD_WEBHOOK_RETURN" ]]; then
    export HD_WEBHOOK_RETURN="$(mktemp)"
  fi

  export HD_WEBHOOK_STATUS_CODE="$(curl -s -o "$HD_WEBHOOK_RETURN" -w "%{http_code}" -H "Token: $HD_WEBHOOK_TOKEN" "$urlprefix/$hook?accept_uuid")"
  local ret="$?"

  if [[ "$HD_WEBHOOK_STATUS_CODE" != "200" ]]; then
    echo got status code "$HD_WEBHOOK_STATUS_CODE" >&2
    return 1
  fi

  export HD_EVENT_ID="$(cat "$HD_WEBHOOK_RETURN" | jq -r ".eventID")"
 
  if [[ -z "$HD_SILENT" ]]; then
    echo "$HD_EVENT_ID"
  fi

  return "$ret"
}

function hdwait() {
  local retry=5
  while (( $retry > 0 )); do
    HD_SILENT=1 hdget "events/$HD_EVENT_ID/wait"
    if (( $HD_STATUS_CODE < 300 )) && (( $HD_STATUS_CODE >= 200 )); then
      break
    fi
    sleep 2
    retry="$(( retry - 1 ))"
  done

  if (( $retry == 0 )); then
    if [[ -z "$HD_SILENT" ]]; then
      # display the session results
      cat "$HD_RETURN"; echo # add a new line
    fi
    echo unable to wait for the event >&2
    return 1
  fi

  while [[ "$HD_STATUS_CODE" == "202" ]]; do
    HD_SILENT=1 hdget "events/$HD_EVENT_ID/wait"
  done

  if [[ -z "$HD_SILENT" ]]; then
    # display the session results
    cat "$HD_RETURN"; echo # add a new line
  fi

  if [[ "$HD_STATUS_CODE" != "200" ]]; then
    echo "got status code $HD_STATUS_CODE" >&2
    return 1
  fi

  local JQ_FILTER=".[].sessions | (.[].status, .[].exported.job_status?)"
  local JQ_RESULTS="$(cat "$HD_RETURN" | jq -r "$JQ_FILTER")"

  export HD_SESSION_SUCCESS="$(echo "$JQ_RESULTS" | grep -c 'success')"
  export HD_SESSION_FAILURE_ERROR="$(echo "$JQ_RESULTS" | grep -c 'failure\|error')"
}
