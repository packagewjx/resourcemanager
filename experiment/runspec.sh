specdir=$1
shellName=$0
currentDir=$(pwd)
runlist="500 502 505 520 523 525 531 541 548 557"

function usage() {
  echo "$shellName <specdir>"
}

if [ -z $specdir ]; then
  usage
  exit
fi

cd $specdir
source ./shrc
for i in $runlist; do
  runcpu --action=onlyrun --config=shiyan --size=test --tune=ref $i > $currentDir/$i.log &
done
