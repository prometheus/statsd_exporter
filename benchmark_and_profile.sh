#!/bin/sh

set -euf

cd "$(dirname "$0")"

workdir="run_$(date -u '+%Y-%m-%d_%H-%M-%S')"

mkdir -p "output/${workdir}"

go test -c -o "output/${workdir}/statsd_exporter.test"
cd "output/${workdir}"

./statsd_exporter.test -test.bench . -test.benchmem | tee benchmark.out

for benchmark in $(awk '$1 ~ /BenchmarkGather/ { sub("-4$", "", $1); print $1 }' benchmark.out)
do
  fname="$(echo "${benchmark}" | tr / _)"
  ./statsd_exporter.test -test.bench "${benchmark}" -test.cpuprofile "${fname}.cpu.pb.gz" -test.memprofile "${fname}.mem.pb.gz"
done

cd ..

tar -cvf "${workdir}.tar.gz" "${workdir}"
