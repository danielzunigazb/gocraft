#!/bin/bash
# Runs on the instance (as root, through SSM): unpacks gocraft-bench.tar.gz into a folder of its own, measures, cleans up.
# It never touches /opt/minecraft: it runs at nice 19 on 127.0.0.1:25570 for under two minutes, and gives up at once
# if the machine is already busy.
#   BASE=/opt/gocraft-bench TAR=/tmp/gocraft-bench.tar.gz run-on-ec2.sh
set -u
BASE=${BASE:-/opt/gocraft-bench}
TAR=${TAR:-/tmp/gocraft-bench.tar.gz}
PORT=${PORT:-25570}
BOTS=${BOTS:-4}
SECS=${SECS:-20}

echo "== machine"
nproc; uname -m
grep -m1 'model name' /proc/cpuinfo || true
free -m | sed -n 1,2p
load1=$(cut -d' ' -f1 /proc/loadavg)
echo "load average now: $(cut -d' ' -f1-3 /proc/loadavg)"
if awk -v l="$load1" -v m="${LOADMAX:-1.2}" 'BEGIN{exit !(l > m)}'; then
	echo "ABORT: the machine is busy (load $load1 > ${LOADMAX:-1.2}); not adding to it"
	exit 2
fi
echo "== the Minecraft server before (java CPU% and RSS, one sample)"
ps -C java -o pid,pcpu,rss,etime,args --no-headers 2>/dev/null | cut -c1-110 || true

rm -rf "$BASE" && mkdir -p "$BASE" && tar -xzf "$TAR" -C "$BASE" || { echo "could not unpack $TAR"; exit 1; }
cd "$BASE" || exit 1
trap 'kill ${SRV:-0} 2>/dev/null; cd /; rm -rf "$BASE"' EXIT

echo "== terrain generation, one chunk (BenchmarkChunk)"
nice -n 19 ./worldgen.test -test.run XXX -test.bench BenchmarkChunk -test.benchtime 20x 2>&1 | grep -E "cpu:|Benchmark"

echo "== server with $BOTS walking bots, ${SECS}s"
nice -n 19 ./gocraft-server -addr 127.0.0.1:$PORT -seed 1234 -view 8 >server.log 2>&1 &
SRV=$!
sleep 1
nice -n 19 ./gocraft-load -addr 127.0.0.1:$PORT -bots "$BOTS" -seconds "$SECS" &
LOAD=$!
sleep $((SECS / 2))
echo "-- halfway: load $(cut -d' ' -f1-3 /proc/loadavg); server $(ps -p $SRV -o pcpu=,rss= | tr -s ' ')  (CPU%, RSS kB)"
ps -C java -o pcpu=,rss= --no-headers 2>/dev/null | sed 's/^/-- minecraft java (CPU%, RSS kB): /' | head -1
wait $LOAD
echo "-- server peak memory: $(grep VmHWM /proc/$SRV/status | tr -s ' \t' ' ')"
echo "-- load average after: $(cut -d' ' -f1-3 /proc/loadavg)"
echo "== done; cleaning up $BASE"
