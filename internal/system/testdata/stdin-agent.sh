#!/bin/sh

IFS= read -r response
printf 'stdin=%s\n' "$response"
