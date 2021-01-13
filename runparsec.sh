#!/usr/bin/env bash

# 测试参数
package=ferret
benchInput=simlarge
suite=parsec
threadCount=1
parsecPackages="blackscholes bodytrack canneal dedup facesim ferret fluidanimate freqmine raytrace streamcluster swaptions vips x264"
benchInput="test simdev simsmall simmedium simlarge native"

# 启动容器制造环境
cid=$(docker run -d --rm --entrypoint sleep spirals/parsec-3.0 3600)
rootDir=$(docker inspect -f "{{.GraphDriver.Data.MergedDir}}" $cid)

function usage() {
  echo "$0 [-p package] [-b benchInput] [-t threadCount]"
  echo "package in [ $parsecPackages ]"
  echo "benchInput in [ $benchInput ]"
}

function abort() {
  docker kill $cid >/dev/null
  echo "aborted"
  exit
}

trap abort INT
trap abort QUIT
trap abort TERM

while getopts "p:b:hs:t:" opt; do
  case $opt in
  b)
    benchInput=$OPTARG
    ;;
  p)
    package=$OPTARG
    ;;
  s)
    suite=$OPTARG
    ;;
  t)
    threadCount=$OPTARG
    ;;
  h | *)
    usage
    exit
    ;;
  esac
done

if [[ ! ${parsecPackages[*]} =~ $package ]]; then
  echo "package should be in [ $parsecPackages ]"
  abort
fi

runCmd="./run -a run -S $suite -p $package -i $benchInput -n $threadCount"

sudo sh -c "cd $rootDir/home/parsec-3.0 && $runCmd && docker kill $cid > /dev/null"
