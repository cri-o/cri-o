#!/bin/sh
echo $@ >> HOOKSCHECK
read line
echo $line >> HOOKSCHECK
