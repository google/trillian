# How To Freeze a Log (CT Example)

## Prerequisites

Some of the tools and metrics that will be used were added in the `v1.0.8`
release. Ensure that your have upgraded to this release or later. If
using MySQL storage ensure that the database schema is updated to at least
that of `v1.0.8`.

The `log_signer` process(es) must be exporting metrics so the queue state
and sequencing can be monitored to [check](#monitor-queue--integration)
that all pending entries have been integrated. Check that their
`--http_endpoint` flag is set to an appropriate value. If it's empty then
update the configuration appropriately and restart them before proceeding.

We will assume that the log tree to be frozen is one that's being used
by Trillian CTFE to serve a Certificate Transparency log. If this is
not the case then consult the documentation for the appropriate application.

## Preparation

### Find the Log ID

Obtain the ID of the tree that is backing the log that is to be frozen. This
can be found in the CTFE config file. Locate the section of the config
file that matches the log to be frozen and pull out the value of `log_id`.
For example with the following config the `log_id` to use would be `987654321`.

```
config {
	log_id: 987654321
	prefix: "the_name_of_the_log"
	roots_pem_file: "... roots file name ...."
	public_key: {
		der: ".... bytes of the public key"
	}
	private_key: {
		[type.googleapis.com/keyspb.PrivateKey] {
			der: ".... bytes of the private key ...."
		}
	}
}
```

### Setup Environment

Build the `updatetree` command if this hasn't already been done and ensure
that it is on your `PATH`.

```
go install github.com/google/trillian/cmd/updatetree
export PATH=${GOPATH}/bin:$PATH
```

Set environment variables to the correct log_id and metrics HTTP endpoint.
For example:

```
LOG_ID=987654321
LOG_SERVER_RPC=localhost:8090
METRICS_URI=http://signer-1:8091/metrics
```

## Set Log Tree To Draining State

Use `updatetree` to set the log tree to a `DRAINING` state.

`updatetree --admin_server=${LOG_SERVER_RPC} --tree_id=${LOG_ID} --tree_state=DRAINING`

Make sure the above command succeeds. At this point the log will not
accept new entries but there may be some that have already been
submitted but not yet integrated.

## Monitor Queue / Integration

If you have monitoring dashboards showing signer mastership e.g. in
Prometheus then this information might be easily available and you
may already have a global view of the state of all the Trillian
servers in the etcd cluster. For the rest of the document we will assume
that this is not the case.

The necessary information can be obtained from the raw metrics
that the server exports. Note that it is possible for elections /
resignations or cluster operations to change the signer responsible for a
tree during the following process. So if you are using metrics directly
from servers be aware that this could happen while you're watching the queue.

Wait until you're sure that the log has finished integrating the
queued leaves. This will be indicated by an incrementing count of
signer runs for the tree, no increase in errors for the tree and zero
leaves being processed for the tree by the signer. The following example
should make this clear.

### Find The Signer

Monitor the statistics available on ${METRICS_URI}. For example:

`curl ${METRICS_URI} | grep ${LOG_ID} | grep -v delay | grep -v latency | grep -v quota`

This might produce output similar to this:

```
entries_added{logid="987654321"} 54
is_master{logid="987654321"} 1
known_logs{logid="987654321"} 1
master_resignations{logid="987654321"} 7
sequencer_batches{logid="987654321"} 54
sequencer_sequenced{logid="987654321"} 54
sequencer_tree_size{logid="987654321"} 7.095373e+07
signing_runs{logid="987654321"} 54
```

First check that `is_master` is not zero. If it is then one of the other
signers is currently handling the tree. Try the command on `signer-2`
or whatever the next cluster member is called until you find the right one.

### Wait For The Queue To Drain

Next check that there is no entry present for `failed_signing_runs` for
the tree. If there is do not proceed until you understand the cause and
confirm that it has been fixed. If signing is failing and this number is
incrementing then the other metrics will not be reliable.

Then check that `signing_runs` is incrementing for the log along with
`sequencer_batches` and then that `entries_added` and `sequencer_sequenced`
remain static. These are counting the number of leaves integrated into
the log by each signing run. While these values are increasing the queue
is being drained.

Continue to monitor the output from accessing `${METRICS_URI}` until you
are sure that the queue has been drained for the log. Remember to ensure
that `is_master` remains non zero during this time. If not you may have
to go back and find the currently active signer.

For additional safety keep watching the metrics for a further number of 
signer runs until you are are that there is no further sequencing activity 
for the log. Because some of the available storage options use queue
sharding (e.g. CloudSpanner) it is not sufficient to rely on no activity
in a single signer run.

## Set Log Tree To Frozen State

**Warning**: Be sure to have completed the queue monitoring process set out
in the previous section. If there are still queued leaves that have not been
integrated then setting the tree to frozen will put the log on a path to 
exceeding its MMD.

Use `updatetree` to set the log tree to a `FROZEN` state.

`updatetree --admin_server=${LOG_SERVER_RPC} --tree_id=${LOG_ID} --tree_state=FROZEN`

Make sure the above command succeeds. The log is now frozen.
