# tejolote

A highly configurable build executor and observer designed to generate 
signed [SLSA](https://slsa.dev/) provenance attestations about build runs.

## What Does That Mean!?

![SLSA Logo](docs/slsa-logo.png)

If you are not familiar with
[provenance](https://www.tiktok.com/@chainguard_dev/video/7133203786927050027) attestations, think of them as non-falsifiable documents that inform you
users how software was built, mainly:

What went in → What was done to the source code (and by who) → what came out.

A provenance attestation provides users with full transparency to the
build process of the software they consume, allowing them to know where
it came from, how it was built and by who.

## Key Features

Tejolote is designed to observe build systems as they run to gather data
about transformations done to software as it goes through the build process.
It features a pluggable model to add more build systems and artifact
storage as the need arises.

* Support for multiple build systems (currently 
[Google Cloud Build](https://cloud.google.com/build), 
[Github Actions](https://github.com/features/actions), 
[Prow](https://github.com/kubernetes/test-infra/tree/master/prow) 
coming soon).
* Support for gathering attestation data in multiple stages or observing a build
while it runs.
* Collection of artifacts from different sources (build system native, 
directories, OCI registries, Google Cloud Storage buckets).
* Attestation signing using [sigstore](https://sigstore.dev)
* Attaching attestations to container images as cosign

## Operational Model

Tejolote watches your build system build (or transform) your software
project. It treats your build as a black box and makes no assumptions as
to the security of the build itself.

It will trust the inputs you tell it to consider and the artifacts your
build produces by looking a the location you instruct it too look for them. 

```mermaid
flowchart LR

 subgraph Build System
   direction LR
   clone("Clone Repository") --> build(Run Build) --> publish(Publish Artifacts)
   fetch("Fetch Materials") --> build
   publish --> oci(Container Registry)
   publish --> gcs(GCS Bucket)
   publish --> file(Filesystem)
end
subgraph Tejolote
  direction LR
  watch(Watch Build System) --> attest(Attest) --> sign(Sign)
watch-. RECORD .-o clone
watch-. RECORD .-o fetch
watch-. CONTINOUSLY OBSERVE .-o build
watch-. COLLECT .-o publish
end

```

While build systems can themselves provide information about the
artifacts produced after a run, Tejolote sits one level above and
will expect artifacts to appear in the storage location(s) you
tell it to monitor.

## Example

Let's say for example you want to attest a Cloud Build job that produces
a bunch of binaries in a GCS bucket. In this case, the gcb project is
`example-project` and artifacts are uploaded to the bucket `test-bucket`
in the directory `/test`:

```bash
tejolote attest \
   gcb://kubernetes-release-test/3190d867-f2e5-4969-aafd-0117b6c8ed12 \
   --artifacts=gs://ulabs-cloud-tests/test/
```

These are made up examples, but Tejolote would produce an attestation
similar to this:

```json
{
  "_type": "https://in-toto.io/Statement/v0.1",
  "predicateType": "https://slsa.dev/provenance/v0.2",
  "subject": [
    {
      "name": "gs://ulabs-cloud-tests/test/bom-windows-amd64.exe",
      "digest": {
        "sha256": "c03c50f220b095bf52a0ca496989a6c07f198d03cb8aad19834df143625ee821"
      }
    }
  ],
  "predicate": {
    "builder": {
      "id": ""
    },
    "buildType": "https://cloudbuild.googleapis.com/CloudBuildYaml@v1",
    "invocation": {
      "configSource": {}
    },
    "buildConfig": {
      "steps": [
        {
          "image": "gcr.io/cloud-builders/git",
          "arguments": [
            "clone",
            "https://github.com/kubernetes/release"
          ]
        },
...
```

Both build system runs and artifact repositories are specified by using
[spec urls](docs/spec-urls.md) that point to the specific runs and storage
location. Check out the 

## What's with the name?

Tejolote /ˌteɪhəˈloʊteɪ/ : From the nahua word _texolotl_. 

![molcajete and tejolote](docs/molcajete.jpg)

A tejolote is the handle of the [_molcajete_](https://en.wikipedia.org/wiki/Molcajete), the prehispanic mortar used to make 
salsa.

So, the idea is to use tejolote to get some salsa out of your project :)
