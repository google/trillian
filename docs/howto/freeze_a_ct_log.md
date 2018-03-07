# How To Freeze a CT Log

## Prerequisites

Some of the tools and metrics that will be used were only added recently.
Before starting ensure that your server is upgraded to a release built
after *fill this in when we know*.

## Preparation

Obtain the ID of the tree that is backing the log that is to be frozen. This
can be found in the CTFE config file. Locate the section of the config
file that matches the one to be frozen and pull out the value of `log_id`.
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

Build the `updatetree` command if this hasn't already been done and ensure
that it is on your `PATH`.

```
go install github.com/google/trillian/cmd/updatetree
export PATH=${GOPATH}/bin:$PATH
```

## Set Log Tree To Draining State

Use `updatetree` to set the log tree to a `DRAINING` state.

`updatetree --tree_id=987654321 --tree-state=DRAINING`

Make sure the above command succeeds. At this point the log will not
accept new entries but there may be some that have already been
submitted but not yet integrated.

## Monitor Queue / Sequencing

Locate the `log_signer` that is currently responsible for signing the tree
that was just modified. Note that it is possible for this to change
during the following process so if you are using metrics directly
from servers be aware that this could happen.

If you have monitoring dashboards showing signer mastership e.g. in
Prometheus then this information might be easily available.

If not it can be obtained from the metrics that the server exports.

Next wait until you're sure that the log has finished integrating the
queued leaves. This will be indicated by an incrementing count of
signer runs for the tree, no increase in errors for the tree and zero
leaves being processed for the tree by the signer.




## Set Log Tree To Frozen State

Use `updatetree` to set the log tree to a `FROZEN` state.

`updatetree --tree_id=987654321 --tree-state=FROZEN`

Make sure the above command succeeds. The log is now frozen.
