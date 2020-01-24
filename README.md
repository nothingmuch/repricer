# Repricer Service

## Design

The service is implemented as a single, distroless Go container, with no
external dependencies, built on Azure Pipelines and deployed to an Azure
Kubernetes Service cluster.

The `endpoints` package has 3 `net/http.Handler`s for the separate endpoints, as
their implementation did not overlap by much (only `product` and `query`
responses share some serialization logic for the response body).

The handlers' constructors need a model to store the applications state,
specified as a Go `interface`, and this is implemented in the `storage`
package.

The storage model is built in two parts, one component to linearize operations
and fill in the `previousPrice` field with *consistent* values, and one
component to aggregate new entries into separate files as specified.

## My Approach and Experience

As my "showing off" I decided to try Azure Devops and Kubernetes, neither of
which I had used before. For my own benefit, I decided not to take into
consideration any time spent learning these technologies, and only limit my
coding time to several hours (which also ran over budget, due to a redesign).

I also suspended disbelief about the task, pretending that I was a new hire to
some company (i.e. I have other team members and need to collaborate with them
using standard tools and practices), and the requirements are "real" business
requirements but that I don't understand the reasons.

As such, I made a deliberate effor to focus on quality/robustness (e.g. avoiding
edge cases, defining clear consistency properties for the data model, etc), like
a real product would need to, mainly because this gave me a clear direction to
pursue for the devops related tasks.

Since the app is stateful and keeps data in memory but also writes it to disk it
must be deployed as a single instance application and avoid rolling deployments,
which added some interest and provoked me to reconsider my design (the first
iteration of the storage model loaded all data on startup, but due to the
`previousPrice` field requirements - which I have to say are [kind of
ridiculous](https://github.com/nothingmuch/repricer/issues/22)).

To avoid these "production" issues, I redesigned the storage model (the initial
design worked in 2 phases, writing records to a staging area first, and then
moving those to the `results` directory when all `previousPrice` fields have
been populated, which to ensure consistency needed snapshot reads of last
prices, contrary to the `product` endpoint's unspecified read consistency
semantics).

## Caveats

Since I took too long on this there are still some things that are important,
and which I would not consider acceptable in a real production system: there's
almost no logging, monitoring or observability.

I had some issues with Azure Monitor which in hindsight seemed to have just been
delays, but I did not revisit that or Azure Application Insights even though I
intended to.

Most critically, some errors are simply discarded, since there's nowhere in the
code to send them too for logging/reporting in the current implementation.

## Deployed Version

The [repository](https://github.com/nothingmuch/repricer) is mirrored to the
`repricer` repostory here, but
[issues](https://github.com/nothingmuch/repricer/issues) and [pull
requests](https://github.com/nothingmuch/repricer/pulls?utf8=%E2%9C%93&q=is%3Apr)
are hosted on GitHub (in hindsight I should have used Azure Devops repos, but I
realized too late, and there's no easy solution to import that data).

The master branch version of the service is deployed to AKS using Pipelines at
[http://51.137.6.251/api](http://51.137.6.251/api/). Note that this is just the
environment set up by the build pipeline, not a separate release pipeline..

## Building & Running

To test the app there are several options:

- `go run main.go` - the app will bind port 8080, and write the `results`
  subdirectory (and an additional `results_by_product` subdirectory next to it)
- `docker build .` will produce a distroless image that runs the service in
  `/tmp/repricer`.
- A Kubernetes deployment based on the Pipelines provided examples is defined in
  the `manifests` subdirectory, but it uses Azure Disks for storage so the
  persistent volume claim only makes sense on AKS.

## Notes for Reviewer

### Throttling

Although I have implemented limiting of the maximum number of concurrent
requests, I had difficulties actually confirming these limits are enforced
without adding e.g. `time.Sleep(10 * time.Millisecond)` to the HTTP handlers.

Secondly `GET` endpoints have rate limiting implemented as a simple
`http.Handler` middleware (with separate token buckets for each endpoint), but
the `reprice` endpoint is rate limited by the data model's nonblocking API,
which queues up to 50 unwritten entries.

### Unit Tests

I deviated from the requirement document specified to *only* test one component.

Initially I had set aside the storage, but then it became apparent that it can't
really be considered a single component, even though it's a single Go package.

As a result of this the `storage` package is [not very comprehensively unit
tested](https://github.com/nothingmuch/repricer/issues/25), and arguably
multiple components have been tested in spite of the clear instructions not to
do that.

Normally I often use a test driven approach, and even if not I try to aim for
100% coverage in unit tests where possible as well as a clear distinction
between unit tests, regression unit tests, and functional/integration tests.

### Github vs. Azure Devops mirror of repository

I started with an Azure Pipelines tutorial, and did not realize Azure Devops
supports hosting repositories until after I got it working with a Github
repository as per the instructions. To review pull requests & issues, make sure
to look in the Github copy.

Secondly, commits generally all have long messages, which azure devops doesn't
like to make very obvious to document the reasoning for the change as well as
any caveats

### Testing in Kubernetes

Since the docker container is distroless, there's no easy way to inspect
`/tmp/repricer` in the cluster environment (`kubectl cp` requires `tar`).

I think the correct approach would be to create a new pod for testing, without
the persistent volume simulating the production environment (e.g. using
`emptyDirectory`, see also
[#26](https://github.com/nothingmuch/repricer/issues/26)).

Of course it's very easy to test the docker image locally using a volume mounted
in `/tmp/repricer`
