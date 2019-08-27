#!/bin/bash
redis-cli -n 3 keys  "*" | while read LINE ; do TTL=`redis-cli ttl "$LINE"`; if [ $TTL -eq  -1 ]; then echo "$LINE"; fi; done;

