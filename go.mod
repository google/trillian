module github.com/google/trillian

go 1.21

require (
	bitbucket.org/creachadair/shell v0.0.8
	cloud.google.com/go/spanner v1.61.0
	contrib.go.opencensus.io/exporter/stackdriver v0.13.14
	github.com/apache/beam/sdks/v2 v2.56.0
	github.com/cockroachdb/cockroach-go/v2 v2.3.8
	github.com/fullstorydev/grpcurl v1.8.9
	github.com/go-redis/redis v6.15.9+incompatible
	github.com/go-sql-driver/mysql v1.8.1
	github.com/golang/mock v1.6.0
	github.com/google/btree v1.1.2
	github.com/google/go-cmp v0.6.0
	github.com/google/go-licenses v1.6.0
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0
	github.com/letsencrypt/pkcs11key/v4 v4.0.0
	github.com/lib/pq v1.10.9
	github.com/prometheus/client_golang v1.19.0
	github.com/prometheus/client_model v0.6.1
	github.com/pseudomuto/protoc-gen-doc v1.5.1
	github.com/transparency-dev/merkle v0.0.2
	go.etcd.io/etcd/client/v3 v3.5.13
	go.etcd.io/etcd/etcdctl/v3 v3.5.13
	go.etcd.io/etcd/server/v3 v3.5.13
	go.etcd.io/etcd/v3 v3.5.13
	go.opencensus.io v0.24.0
	golang.org/x/crypto v0.23.0
	golang.org/x/sync v0.7.0
	golang.org/x/sys v0.20.0
	golang.org/x/tools v0.20.0
	google.golang.org/api v0.176.1
	google.golang.org/genproto v0.0.0-20240401170217-c3f982113cda
	google.golang.org/genproto/googleapis/api v0.0.0-20240415180920-8c6c420018be
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240415180920-8c6c420018be
	google.golang.org/grpc v1.63.2
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.3.0
	google.golang.org/protobuf v1.34.1
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/klog/v2 v2.120.1
)

require (
	cloud.google.com/go v0.112.2 // indirect
	cloud.google.com/go/auth v0.3.0 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.2 // indirect
	cloud.google.com/go/compute/metadata v0.3.0 // indirect
	cloud.google.com/go/iam v1.1.7 // indirect
	cloud.google.com/go/longrunning v0.5.6 // indirect
	cloud.google.com/go/monitoring v1.18.1 // indirect
	cloud.google.com/go/profiler v0.4.0 // indirect
	cloud.google.com/go/storage v1.39.1 // indirect
	cloud.google.com/go/trace v1.10.6 // indirect
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/GoogleCloudPlatform/grpc-gcp-go/grpcgcp v1.5.0 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/Masterminds/sprig v2.22.0+incompatible // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/aws/aws-sdk-go v1.51.8 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bgentry/speakeasy v0.1.0 // indirect
	github.com/bufbuild/protocompile v0.6.0 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/census-instrumentation/opencensus-proto v0.4.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cncf/xds/go v0.0.0-20240325133356-033564927e4a // indirect
	github.com/coreos/go-semver v0.3.1 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.3 // indirect
	github.com/distribution/reference v0.5.0 // indirect
	github.com/docker/docker v25.0.5+incompatible // indirect
	github.com/docker/go-connections v0.5.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/envoyproxy/go-control-plane v0.12.0 // indirect
	github.com/envoyproxy/protoc-gen-validate v1.0.4 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/gofrs/flock v0.8.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt/v4 v4.5.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/licenseclassifier v0.0.0-20210722185704-3043a050f148 // indirect
	github.com/google/pprof v0.0.0-20240320155624-b11c3daa6f07 // indirect
	github.com/google/s2a-go v0.1.7 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.2 // indirect
	github.com/googleapis/gax-go/v2 v2.12.3 // indirect
	github.com/gorilla/websocket v1.5.1 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.16.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.19.1 // indirect
	github.com/huandu/xstrings v1.2.0 // indirect
	github.com/imdario/mergo v0.3.16 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jackc/chunkreader/v2 v2.0.1 // indirect
	github.com/jackc/pgconn v1.14.3 // indirect
	github.com/jackc/pgio v1.0.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgproto3/v2 v2.3.3 // indirect
	github.com/jackc/pgservicefile v0.0.0-20231201235250-de7065d80cb9 // indirect
	github.com/jackc/pgtype v1.14.3 // indirect
	github.com/jackc/pgx/v4 v4.18.3 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/jhump/protoreflect v1.15.3 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/jonboulle/clockwork v0.4.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kevinburke/ssh_config v0.0.0-20190725054713-01f96b0aa0cd // indirect
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/miekg/pkcs11 v1.1.1 // indirect
	github.com/mitchellh/copystructure v1.0.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/mwitkow/go-proto-validators v0.2.0 // indirect
	github.com/olekukonko/tablewriter v0.0.5 // indirect
	github.com/onsi/ginkgo v1.16.5 // indirect
	github.com/onsi/gomega v1.29.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.0 // indirect
	github.com/otiai10/copy v1.6.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/common v0.51.1 // indirect
	github.com/prometheus/procfs v0.13.0 // indirect
	github.com/prometheus/prometheus v0.51.0 // indirect
	github.com/pseudomuto/protokit v0.2.0 // indirect
	github.com/rivo/uniseg v0.4.4 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sergi/go-diff v1.3.1 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/soheilhy/cmux v0.1.5 // indirect
	github.com/spf13/cobra v1.8.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/src-d/gcfg v1.4.0 // indirect
	github.com/tmc/grpc-websocket-proxy v0.0.0-20220101234140-673ab2c3ae75 // indirect
	github.com/urfave/cli v1.22.14 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	github.com/xiang90/probing v0.0.0-20221125231312-a49e3df8f510 // indirect
	go.etcd.io/bbolt v1.3.9 // indirect
	go.etcd.io/etcd/api/v3 v3.5.13 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.13 // indirect
	go.etcd.io/etcd/client/v2 v2.305.13 // indirect
	go.etcd.io/etcd/etcdutl/v3 v3.5.13 // indirect
	go.etcd.io/etcd/pkg/v3 v3.5.13 // indirect
	go.etcd.io/etcd/raft/v3 v3.5.13 // indirect
	go.etcd.io/etcd/tests/v3 v3.5.13 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.49.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.49.0 // indirect
	go.opentelemetry.io/otel v1.24.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.24.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.24.0 // indirect
	go.opentelemetry.io/otel/metric v1.24.0 // indirect
	go.opentelemetry.io/otel/sdk v1.24.0 // indirect
	go.opentelemetry.io/otel/trace v1.24.0 // indirect
	go.opentelemetry.io/proto/otlp v1.1.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/exp v0.0.0-20240325151524-a685a6edb6d8 // indirect
	golang.org/x/mod v0.17.0 // indirect
	golang.org/x/net v0.24.0 // indirect
	golang.org/x/oauth2 v0.19.0 // indirect
	golang.org/x/text v0.15.0 // indirect
	golang.org/x/time v0.5.0 // indirect
	golang.org/x/xerrors v0.0.0-20231012003039-104605ab7028 // indirect
	gopkg.in/cheggaaa/pb.v1 v1.0.28 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	gopkg.in/retry.v1 v1.0.3 // indirect
	gopkg.in/src-d/go-billy.v4 v4.3.2 // indirect
	gopkg.in/src-d/go-git.v4 v4.13.1 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	sigs.k8s.io/yaml v1.4.0 // indirect
)
