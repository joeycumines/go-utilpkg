module github.com/joeycumines/go-sql/export/mysql

go 1.21

toolchain go1.21.6

// To update, identify TIDB_HASH (probably from a release branch), delete
// go.sum and all dependencies listed in go.mod, then run:
//
// go get -u github.com/pingcap/tidb/pkg/parser@$TIDB_HASH
// go get -u github.com/pingcap/tidb@$TIDB_HASH
// go mod tidy
//
// OR (if override is necessary), reset it to just:
//
// replace github.com/pingcap/tidb/pkg/parser => github.com/joeycumines/tidb/pkg/parser latest
// replace github.com/pingcap/tidb => github.com/joeycumines/tidb latest
//
// Then run:
//
// go mod tidy

require (
	github.com/go-test/deep v1.1.0
	github.com/hexops/gotextdiff v1.0.3
	github.com/joeycumines/go-sql v0.0.0-20240121153107-ec42efde5c12
	github.com/pingcap/tidb v1.1.0-beta.0.20240122141050-52794d985ba6
	github.com/pingcap/tidb/pkg/parser v0.0.0-20240122141050-52794d985ba6
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cloudfoundry/gosigar v1.3.6 // indirect
	github.com/cockroachdb/errors v1.8.1 // indirect
	github.com/cockroachdb/logtags v0.0.0-20190617123548-eb05cc24525f // indirect
	github.com/cockroachdb/redact v1.0.8 // indirect
	github.com/cockroachdb/sentry-go v0.6.1-cockroachdb.2 // indirect
	github.com/coreos/go-semver v0.3.1 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/cznic/mathutil v0.0.0-20181122101859-297441e03548 // indirect
	github.com/dgryski/go-farm v0.0.0-20200201041132-a6ae2369ad13 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/btree v1.1.2 // indirect
	github.com/google/uuid v1.4.0 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0 // indirect
	github.com/joeycumines/go-catrate v0.0.0-20240113051756-0b19f6e64c0b // indirect
	github.com/joeycumines/go-detect-cycle v1.0.1 // indirect
	github.com/joeycumines/logiface v0.5.0 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20230326075908-cb1d2100619a // indirect
	github.com/matttproud/golang_protobuf_extensions/v2 v2.0.0 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/pingcap/errors v0.11.5-0.20231212100244-799fae176cfb // indirect
	github.com/pingcap/failpoint v0.0.0-20220801062533-2eaa32854a6c // indirect
	github.com/pingcap/kvproto v0.0.0-20231226064240-4f28b82c7860 // indirect
	github.com/pingcap/log v1.1.1-0.20230317032135-a0d097d16e22 // indirect
	github.com/pingcap/sysutil v1.0.1-0.20230407040306-fb007c5aff21 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/power-devops/perfstat v0.0.0-20221212215047-62379fc7944b // indirect
	github.com/prometheus/client_golang v1.18.0 // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.45.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rogpeppe/go-internal v1.11.0 // indirect
	github.com/shirou/gopsutil/v3 v3.23.10 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/tiancaiamao/gp v0.0.0-20221230034425-4025bc8a4d4a // indirect
	github.com/tikv/client-go/v2 v2.0.8-0.20231227070846-61c486af13a5 // indirect
	github.com/tikv/pd/client v0.0.0-20240109100024-dd8df25316e9 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/twmb/murmur3 v1.1.6 // indirect
	github.com/yusufpapurcu/wmi v1.2.3 // indirect
	go.etcd.io/etcd/api/v3 v3.5.10 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.10 // indirect
	go.etcd.io/etcd/client/v3 v3.5.10 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.26.0 // indirect
	golang.org/x/exp v0.0.0-20240119083558-1b970713d09a // indirect
	golang.org/x/net v0.19.0 // indirect
	golang.org/x/sync v0.5.0 // indirect
	golang.org/x/sys v0.16.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	google.golang.org/genproto v0.0.0-20231211222908-989df2bf70f3 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20231120223509-83a465c0220f // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20231212172506-995d672761c0 // indirect
	google.golang.org/grpc v1.60.1 // indirect
	google.golang.org/protobuf v1.32.0 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
)
