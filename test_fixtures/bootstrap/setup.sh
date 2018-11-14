#!/bin/sh

set -e

repo_dir=/tmp/honeydipper-test-config

(
  rm -rf $repo_dir
  mkdir -p $repo_dir
  cp -r $(dirname $0)/* $repo_dir/

  cd $repo_dir
  git init .
  git add *
  git -c user.name='circle' -c user.email='circle@nomail.com' commit -m 'init' -a
) &> /dev/null

echo $repo_dir
