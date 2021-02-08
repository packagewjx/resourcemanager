$2 ~ /perlbench_r_base.shiyan-m64|cpugcc_r_base.shiyan-m64|mcf_r_base.shiyan-m64|\
omnetpp_r_base.shiyan-m64|x264_r_base.shiyan-m64|deepsjeng_r_base.shiyan-m64|\
leela_r_base.shiyan-m64|exchange2_r_base.shiyan-m64|xz_r_base.shiyan-m64|cpuxalan_r_base.shiyan-m64/ \
{split($2,cmd,"/"); print "perf stat -e instructions,cycles,cache-references,cache-misses -p " $1 " -o " cmd[length(cmd)] "." $1 ".csv -x , -- sleep 60 &"| "/bin/bash"}