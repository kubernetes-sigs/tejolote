# PubSub Support

Tejolote supports starting an attestation and messaging itself via pubsub 
to collect data after a run is done.

## Sleeping and Resuming

TBD

## Recieving Data When Attestting

Data communicated from the `tejolote start attestation` invocation will
arrive base64 encoded. In order to rebuild the data, tejolote includes two
hidden flags:

```
  --encoded-attestation=""
  --encoded-snapshots=""
```

These two flags get base base64 encoded data, the first flag (`--encoded-attestation`)
is the partial in-toto attestation to be completed with the finalized
run data.

The second one (`--encoded-snapshots=""`) includes the initial state of the
artifact stores as seen by tejolote before the run.

The flags are intended to be used by automation driving tejolote and therefore
are not visible in the CLI help.