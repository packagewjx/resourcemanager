#!/usr/bin/env bash

# 测试对象定义
package=ferret
benchInput=simlarge
operation=pin
traceCount=500000000
perfEvents="mem_inst_retired.all_loads,mem_inst_retired.all_stores,cpu/event=0xb7,umask=0x01,offcore_rsp=0x801C0003,name=L3Hit/,cpu/event=0xb7,umask=0x01,offcore_rsp=0x84000003,name=L3Miss/,cpu_clk_unhalted.thread,inst_retired.any,cycle_activity.cycles_l3_miss,cycle_activity.cycles_mem_any"
runCmd="./run -a run -S parsec -p $package -i $benchInput >/dev/null 2> /dev/null"
#runCmd="./run -a run -S parsec -p $package -i $benchInput"
currentDir=$(pwd)
outFile=$currentDir"/sampleOut"
cid=$(docker run -d --rm --entrypoint sleep spirals/parsec-3.0 3600)
rootDir=$(docker inspect -f "{{.GraphDriver.Data.MergedDir}}" $cid)
pinToolPath="/home/wjx/Workspace/pin-3.17/source/tools/MemTrace2/obj-intel64/MemTrace2.so"

function getPid() {
  awkScript='BEGIN {IFS=" "} $2 ~ /'"$package"'/ {print $1}'
  echo "$(ps a -o pid,command | awk $awkScript)"
}

if [[ $operation == "perf" ]]; then
  sudo sh -c "cd $rootDir/home/parsec-3.0 && perf stat -e $perfEvents -o $currentDir/$outFile -x , $runCmd && docker kill $cid > /dev/null" &
else
  sudo sh -c "cd $rootDir/home/parsec-3.0 && $runCmd && docker kill $cid > /dev/null" &
  pid=$(getPid)
  while [[ -z $pid ]]; do
    sleep 0.1
    pid=$(getPid)
  done
  sudo /home/wjx/bin/pin -pid $pid -injection dynamic -t $pinToolPath -fifo $outFile -stopat $traceCount
fi

return

