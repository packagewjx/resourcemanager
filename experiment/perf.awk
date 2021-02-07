$2 ~ /perlbench_r_base.shiyan-m64|cpugcc_r_base.shiyan-m64|mcf_r_base.shiyan-m64|\
omnetpp_r_base.shiyan-m64|x264_r_base.shiyan-m64|deepsjeng_r_base.shiyan-m64|\
leela_r_base.shiyan-m64|exchange2_r_base.shiyan-m64|xz_r_base.shiyan-m64/ \
{print "perf stat -e instructions,cycles,cache-references,cache-misses -p " $1 " -- sleep 5 && echo ============================================ &"| "/bin/bash"}