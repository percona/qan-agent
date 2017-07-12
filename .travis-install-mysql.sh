#!/bin/sh


docker run \
  --name mysql \
  -p ${MYSQL_HOST}:${MYSQL_PORT}:3306 \
  -e MYSQL_ALLOW_EMPTY_PASSWORD=yes \
  -d \
  mysql:${MYSQL_VERSION}
