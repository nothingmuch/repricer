# worklog and notes

[comment]: # (Normally I don't take very careful notes, mostly messy/illegible ones on paper. I made a deliberate attempt to articulate my thought process more than I normally would, but to be honest it was a very positive experience. It did slow me down a bit, but I think also ended up saving some time as well, especially by better documenting my mistakes.)

## Initial observations

Tue 14:30

Underspecified requirements:
- nothing about authentication or access control
- 50 requests per second cap implies load expectations, but doesn't describe its
  characteristics (sustained, bursty, thundering herds, peak hours, etc)...
- time zone for RFC 3339 format timestamps (in data) seems arbitrary - assuming
  server localtime is being used (which is not correct for data)
- ~~no time zone for epoch format timestamps (UTC?)~~ it's UTC by definition, so
  there's no ambiguity
- `previousPrice` field when no data available - `null` or omitted?
- length constraints on product ID. implies at least enough for hex UUID string.
- validity constraints for price data (e.g. min/max, precision)
- new file every one second - ~~what about seconds when there's no requests?
  assuming no since that will create disk load even when no new information has
  been acquired~~ at *least* one entry per file
- `from` and `to` fields in `query` endpoint have no data type, assuming
  timestamps because offsets are pretty redundant with paging params

Assuming that it's a sustained but variable load on the `reprice` endpoint
depending on crawl farm described in the document, with large growth potential,
and variable, spikey load on the `product`/`query` endpoints.

Since all requests require model state to be maintained, will follow a kappa
arch inspired design in order to ease imaginary transition to distributed
storage and computation later in this project's lifecycle.

Events ingressed from `reprice` endpoint need to be processed through model
state, since on disk format requires the previous price in events

Dubious about:
- slight moral objection to unit testing *only* one component, even in interview
  setting. specification says i can *unit test* only one component, but am i
  allowed to write *integration* tests?
- load throttling inside of the app seems like a cross cutting concern,
  especially with variable/changing workloads it would seem more appropriate to
  handle this on the infrastructure level
- data model
  - requirement to have product data in memory - assuming this is for simplicity
    in interview setting
  - data at rest format has data dependencies that rely on model state, IMO should
    either be raw inputs (to avoid blocking writes on state updates), or a
    normalized representation with clear consistency guarantees for
    readers/writers. assuming this is for simplicity in interview setting
  - using nominal time (even with uniform time zone) in on disk format implies
    that there is only one ingress point, the HTTP api that adds the timestamps,
    which seems very dubious. for the product/query endpoints to be able to
    present a consistent view using these time values in a distributed setting
    they will need to be assigned by primary storage, or have clear precedence
    ordering based on causal relationships, or at the very least monotonic time
    on a single machine. it seems like a flaw in the model to rely on nominal
    time for total ordering of events in this way as it may present challenges
    to or constrain horizontal scaling possibilities in the future.

## Initial thoughts & assumptions on how to approach task

Tue 15:00

I will suspend disbelief about scenario, assuming most of the specification's
requirements are hard requirements, with justifications for the details on a
need to know basis

I will pretend this is just the start of a long term with possibly changing
requirements and reasonable growth expectations. I will working as if I was
hired to start a new project but all of my teammates, who already use Azure
Devops, are unavailable as I start, but will have to catch up.

I will use Go, because I'm comfortable with it, it is well suited for the
parallelism requirements and the domain, and it is easily deployed in `FROM
scratch` docker containers.

Price data will be interpreted as `json.Number` so that no float conversion or
other loss of precision will happen, and left numerically uninterpreted since
nothing in the specification seems to require that, and the specification has no
numerical constraints, but this is a bit "rosh katan".

For `reprice` requests HTTP frontend will just parse JSON into internal
representation of price events, these will be handed into abstract model for
processing, augmented with previous price data, and written out to windowing
logger.

Not sure how to handle model updates with respect to write failures to the log,
might make simplifying assumption that log writes always succeed (i.e. crash
entire service if they fail and rely on rate limiting to prevent that from
happenning). This seems reasonable since consistency requirements are
underspecified.

---

edit to add:
202 status doesn't guarantee processing which entails that it doesn't guarantee
durable writes, so actually consistency requirements *are* adequately specified.

a nice service should probably stop accepting requests if too many buffers
remain unwritten/synced, which should be re-attempted using some reasonable
backoff strategy (probably not exponential backoff, since limit is constant).

incidentally since i implemented immediate processing (product endpoint
immediately sees changes even before buffers are flushed), that means read
consistency for product endpoint is not guaranteed.

---

The on disk data preserves the full primary information (with an additional
derived field), so on startup data can just be replayed into the in memory model
to restore the last state.

Not sure if a simple checkpointing mechanism to avoid O(N) startup costs, where
N is the number of past `reprice` requests, would be appropriate for something
simple, leaning towards no.

Another way to be to read the events backwards on demand until a required
productId is found, but this can lead to unpredictable load when a `reprice`
request hits a cold productId, or in the worst case a non existent one, causing
a recently started web worker to do a full scan before it can handle that
request, but this should not block other requests.

Unfortunately this means both write and read requests may block, either due to
cold start, or with erratically degraded performance if fast start is preferred.

Even in a cloud environment with load balancing, blue/green deployments etc
neither seems more appropriate, since even though full scan on startup gives
more predictable request performance for all product IDs, in that environment it
seems contrived as it'd probably be using a distributed database with a
normalized model in order to have fast starts even for large N.

Current thoughts are that lazy backwards seems simplest, demonstrates writing
correct Go in distributed setting, but might fall back on full scan if this
turns out to be too complicated.

The `product` endpoint will query the same model state that `reprice` depends
on, but `query` handles full logs.

---

15:50 wrote email with nitpicky questions

---

~evening-night
- thought more about data model, `previousPrice` introduces a lot of complexity...
- looked a bit at Azure Devops, thinking of using CI/CD & AKS

## Initial thoughts on ops

Spent some time learning about Kubernetes in Azure, will attempt to build entire
appiteratively using Azure Devops workflow as best I can.

Current plan is to build and deploy the repository to AKS using pipelines.

- Build: use latest golang official docker image for build, `from scratch`
  static output, go mod for dependencies (which shouldn't really exist)
- Deploy:
  - app on 80:8080 ? what's the default?
  - ReadWriteOnce in /tmp/repricer/ - (but not initially)
  - ephemeral storage for /tmp/repricer/cache
  - prometheus on port ??? - azure monitoring defaults
  - liveness, readiness, go profile on port ~~9080~~9102 - segregate from service mux

## Azure Devops

Wed ???
(written Thu morning)

Spent some time on Wednesday learning about Azure Devops. Also wrote a stub HTTP
handler in order to try out Azure pipeline with following (non-committal)
learning goals in mind:

1. [x] Build minimal reproducible go docker container from project in CI Azure Pipeline
   - [x] pipeline vm image needs Docker. recent ubuntu LTS? ~~template is still
         on 16.04~~ [that was a sample repo, default vmImage is ubuntu-latest]
   - [x] build image should probably be go ~~1.12~~ or 1.13 on alpine
   - [x] app image should be a from scratch image with static binary, no SSL
         certs required for now
2. [x] Push container to Azure Container Registry
3. [x] Deploy to Azure Kubernetes Service (in a dev space?)
   - [ ] single node, single replica deployment - still not sure how to enforce
         singleton pattern except with ReadWriteOnce volume, but that kind of
         makes sense (even though it's configured in /tmp i will pretend)
   - [x] add proper healthcheck probes
   - [ ] ensure logging is handled properly (structured logging in app?)
     - what is standard solution? presumably k8s node picks up docker logs?
     - is there an azure service for log management? if is it simplest to send
       to it directly?
4. [x] Try Azure Monitoring
   - [x] ~~minimal prometheus monitoring~~
         ~~couldn't get azure monitor to work~~
         actually it did work in the end
   - [ ] opentracing? spans are not really justified for this service, but
         it'd be fun to see if there's easy to use tooling on azure (seems
         opencensus is supported but i'm not familiar with the spec), and it's
         possible to log spans internally, especially since i think to be done
         correctly, processing of files on disk will have a background component
         to it.

Thu evening

Azure Devops is pretty enjoyable so far. A little more boilerplate heavy then
I'd like, had to read through lots of copypasta'd examples before settling on
what I wanted, the provided go template was remarkably bloated, as were most
other go on azure pipelines examples I found.

I did not attempt to carefully read through AKS deployment and the corresponding
task definitions boilerplate yet). Most of my background knowledge about
kubernetes is due to being interested in its architecture and security, so while
I have an OK-ish conceptual understanding of it I need to fill significant gaps
to understand exactly what the commands are doing.

Video material on Azure Devops and related topics (both short and long) has
generally been low SNR for me, and Azure docs seem to be either very how-to-ish
and therefore lack generality or motivation, or written for for a more
experienced target audience, so my retention is quite low so far. I expect this
should improve with more hands-on experience. I also need to learn to use `az`
on CLI, the web GUI is annoyingly heavy (but it helps to have an explorable &
visual interface for now, so I stuck with it).

### Issues

Didn't run into any trouble until Azure Monitor - which I also found the hardest
to understand.

#### Live Data RBAC

Credentials did not appear under default configuration with Monitor addon
enabled on AKS, clusterUser was not allowed to fetch API events:

> pods "repricer-7d54f5b586-7jvfp" is forbidden: User "clusterUser" cannot get
> resource "pods/log" in API group "" in the namespace "azure-pipelines"

Added the boilerplate config to devops dir.

#### Prometheus

Weirdness of `true` vs. `"true"` in YAML prometheus annotations caused a parse
error from the OMS agent. Very surprising given YAML has support for all of the
field types described. Is it limited by `metadata/annotations`?

Even after apparently fixing that still not seeing any of the exported metrics
(confirmed they do appear in the `/metrics` endpoint).

There are no `prometheus` namespace entries in `InsightsMetrics` table or any
notable events in `KubeMonAgentEvents`. Even if adding
`urls = ["${CONTAINER_IP}:9102/metrics"]` to the ConfigMap, not seeing any data.

Giving up on prometheus based metrics gathering for now, might revisit with
Application Insight SDK, for metrics, structured log events, and distributed
tracing.

... and after splitting healthcheck and prometheus PRs, `InsightMetrics`
contains data gathered from the prometheus only PR, despite no code or
configuration changes as far as I can tell. Will revisit branch afterwards, and
reconsider app insights (maybe both?)

## Sketch/redesign of more RESTful HTTP API:

Thu afternoon

The specified HTTP api handles parameters inconsistently, and does not model
data as resources (data is written and queried out of separate paths), namely
`productId` is supplied in the request body for `reprice` requests, but is part
of the path for `product` requests.

The central resources are prices, which are grouped by products:

- `/price/{priceId}`
  - `GET` historical price information (and so permanently cacheable). `priceId`
    should be UUID or seq# depending on whether or not enumeration is an issue
    (but access control spec was not provided...)
- `/prices`
  - `GET` paged view of all price data with filtering. page{,size}, to, from and
    productId are query parameters.
- `/product/{productId}/price`
  - `POST` update price, body should still be JSON in case additional parameters
    are added (e.g. source of data)
  - `GET` latest price, temporary redirect to `/price/{priceId}` of last price
    (or canonical rel Link header?)
- `/product/{productId}/price/{pos}` 
  - `GET` specific price as specified by `pos` of `productId`. redirect (or just
    canon link?) to canonical `price/priceId`. `pos` could be time value (last
    price before that time), or an integer indicating the n-th update to that
    `product`, with negative offsets meaning previous n-th price
    (`/product/{productId}/price` is like negative 0). if `pos` is an absolute
    reference, redirect can be permanent and cacheable (negative offsets could
    mean previous n-th price, with `/price` endpoint being negative 0)

## Stateful, Persistent storage

Fri afternoon-late night

Code was straightforward.

Main time sink was kubernetes:

---

Ran into trouble with persistent volume management on azure pipelines using
`ReadWriteOnce` mode. Volume attachment seems unreliable but pretty erratically
so. I also think I there are interactions between deployments create pods in
different namespaces which then make claims on persistent volumes and rolling
updates.

---

Suspicion was correct, `strategy: { type: Recreate }` seems to have fixed the
issue.

---

Setting up a persistent volume claim with a
storage class that specifies UID 65534 (which is an ugly hack in and of itself),
it appears that the newly provisioned disk might not be formatted?

>      Warning  FailedMount             34s                kubelet, aks-agentpool-18695058-1  MountVolume.MountDevice failed for volume "pvc-b739994a-39b0-11ea-8a5b-8a76d9b88753" : azureDisk - mountDevice:FormatAndMount failed with mount failed: exit status 32
>    Mounting command: systemd-run
>    Mounting arguments: --description=Kubernetes transient mount for /var/lib/kubelet/plugins/kubernetes.io/azure-disk/mounts/m1484313853 --scope -- mount -t ext4 -o gid=65534,uid=65534,defaults /dev/disk/azure/scsi1/lun0 /var/lib/kubelet/plugins/kubernetes.io/azure-disk/mounts/m1484313853
>    Output: Running scope as unit run-r650ec43d317848f49737e99cfcee37b6.scope.
>    mount: wrong fs type, bad option, bad superblock on /dev/sdc,
>           missing codepage or helper program, or other error
>
>           In some cases useful info is found in syslog - try
>           dmesg | tail or so.
>      Warning  FailedMount  29s  kubelet, aks-agentpool-18695058-1  MountVolume.MountDevice failed for volume "pvc-b739994a-39b0-11ea-8a5b-8a76d9b88753" : azureDisk - mountDevice:FormatAndMount failed with mount failed: exit status 32
>    Mounting command: systemd-run
>    Mounting arguments: --description=Kubernetes transient mount for /var/lib/kubelet/plugins/kubernetes.io/azure-disk/mounts/m1484313853 --scope -- mount -o gid=65534,uid=65534,defaults /dev/disk/azure/scsi1/lun0 /var/lib/kubelet/plugins/kubernetes.io/azure-disk/mounts/m1484313853
>    Output: Running scope as unit run-r961e90197aa74b78a40f64aa818d21fa.scope.
>    mount: wrong fs type, bad option, bad superblock on /dev/sdc,
>           missing codepage or helper program, or other error
>
>           In some cases useful info is found in syslog - try
>           dmesg | tail or so.

---

Seems like I misremembered and uid/gid are not supported options for `ext4`.
PEBCAK is a tautology ;-)

Using a storage class for making `/tmp/repricer` writable to non root process
seems like a really bad idea anyway, it was just the first think i found (in the
context of azure files, not azure disks which seems more appropriate for the
scenario described in the exercise).

Learned about `initContainers`, which led to a much simpler but still kind of
ugly (because because it disperses coupled information) fix - a busybox
init container that chowns the directory to match the non priviliged UID in the
dockerfile.

----

k8s issues were being further compounded by azure container registry
authentication (missing image pull secret?) errors when attempting to `kubectl
apply` changes to the deployment. Still not 100% sure these errors are the
result of that, but I am pretty sure it has worked previously (though perhaps in
`default` namespace instead of PR one created by pipeline?), I suspect the image
was pulled before and cached in my previous attempts, whereas for these when
seeing these failures it's because the pipeline deployment failed, and the
namespace is new:

>    Normal   Pulling                 38s (x4 over 2m)    kubelet, aks-agentpool-18695058-1  Pulling image "repricer.azurecr.io/repricer"
>    Warning  Failed                  37s (x4 over 2m)    kubelet, aks-agentpool-18695058-1  Failed to pull image "repricer.azurecr.io/repricer": rpc error: code = Unknown desc = Error response from daemon: Get https://repricer.azurecr.io/v2/repricer/manifests/latest: unauthorized: authentication required
>    Warning  Failed                  37s (x4 over 2m)    kubelet, aks-agentpool-18695058-1  Error: ErrImagePull
>    Warning  Failed                  22s (x6 over 119s)  kubelet, aks-agentpool-18695058-1  Error: ImagePullBackOff
>    Normal   BackOff                 7s (x7 over 119s)   kubelet, aks-agentpool-18695058-1  Back-off pulling image "repricer.azurecr.io/repricer"

What is the correct approach to manually debugging/investigating? Need to
understand AKS tasks and k8s cluster concepts in more detail, and work out
precise relationships between the various resources. It feels like I need to
learn the names concepts i'm familiar with under different terms, and study the
ones that don't correspond to anything i know. Try to focus what is meant to be
consumed/managed by humans vs. machines, so far i'm finding a lot of overlap
between the two (e.g. ephemeral pods created by deployments vs. longer lived
ones managed by humans in version control, both semantically and syntactically).
This is cooler than I would have expected, but I bet there are some hairy
edge/corner cases even though a lot of things seem to be pretty straightforward
the system as a whole is still complicated.

---

Upon reflection, I should have probably dealt with the UID and working directory
issue in the Dockerfile, and done the minimum possible in the deployment pod
template, good thing I haven't merged the persistence PR yet.

## Concerns about Rate limiting specification

After reviewing the specification, I noticed a possible contradiction.
Previously I thought only the `product` and `query` endpoints need to be rate
limited:

> The product and query endpoints should be able to handle up to 50 parallel
> requests each. In case your service receives more requests it should not
> accept the requests and return an appropriate error code

But the previous page implies the `reprice` endpoint might also be rate limited:

> 100 parallel requests to the server for repricing 50 different products. 
> 
> We expect some requests to fail and no less than 50 to return 200 status code.

Firstly, this should be a 202 status code, not 200 (i guess 2xx is what was
meant?).
 
Secondly, in order to ensure timestamps of entries are monotonically increasing
(for query logs' `to`/`from` inputs to have a coherent meaning), the
timestamping is synchronized with the push to the buffer. This implies that
reprice requests cannot fail due to load, instead they will see contention for
the mutex, and when the buffer is full it is handed off to a goroutine for
writing to disk.

This behaviour could be modified, for example to deny any requests whenever the
buffer is full, instead of detaching it from the `store` object and flushing in
the background, but this seems ill advised since the overheads of triggering a
preemptive flush are low.

For now, I will not interpret
> We expect some requests to fail
as a *requirement* to fail at least one request under such load, but only as
allowing some requests to fail.

I may revisit this if I have time to implement a sparse/lazy last known price
index, which means that `previousPrice` will have to be computed in the
background, which entails some sort of background worker and a job queue. At
that point it makes sense to fail `reprice` requests when the worker is
overloaded.

## Better approach to running as non priviliged user?

Sat evening-late night

A more correct and cleaner approach than non priviliged uid in the Dockerfile
might be to modify main.go so that it unconditionally drops privilges, only
initializing as root any missing directory if its parent directory is owned by
root and non sticky. The uid should be taken from this directory, falling back
to nobody if it's owned by root.

---

There's potentially another good long term reason for it too - xattr support for +i
(immutable) and +a (append only, on the directory)

Setting attrs requires root privs or `CAP_SYS_ADMIN` (possibly only
`CAP_DAC_OVERRIDE`?), so before calling setuid the process can create a unix
domain socket pair and fork, where one of the forks retains root privliges and
apart from creating & chowning storage dir at startup, only sets attrs on file
descriptors depending on whether they are files or directories, and the
unpriviliged process, after finalizing data for a file, sends the synced file
descriptor down the socket only to be made immutable and closed.

---

Disregard those, `CAP_LINUX_IMMUTABLE`, `CAP_FOWNER`, `CAP_SYS_RESOURCE` are the
relevant capabilities.

---

Could also grant `CAP_NET_BIND_SERVICE` to bind port 80 to declutter the
pod/service configuration.

---

`/etc/services` is [not needed](https://golang.org/src/net/port_unix.go#L21) in
order to [resolve `":http"`](https://golang.org/src/net/lookup.go#L44) the
default value used by `net/http.ListenAndServe`, so there should be no problem
with a `FROM scratch` container.

---

`syscall.Setuid` is ineffective in Go if preceded by `net.Listen`, since that
will possibly spawn new OS threads that will not drop priviliges.

https://stackoverflow.com/questions/41248866/golang-dropping-privileges-v1-7

---

Hmm, depending on node host OS, `aufs` might also lack support setcap even if
set on the server binary in the image (I'm not even sure if `COPY` in
dockerfiles preserves xattrs).

https://stackoverflow.com/questions/44117543/getcap-setcap-not-working-in-docker-container-with-debian-stretch-host

---

Learned k8s security contexts and in particular `fsGroup` - this along with
capabilities means no root privs are required to set up a storage volume
writable by a non-priviliged uid.

This seems like the simplest as well as the most concise approach in the k8s
`deployment.yml`, the `Dockerfile`, and `main.go`.

POLA could be more strictly adhered to by forking in `main()` and dropping
capabilities in different processes. Seems overkill in an otherwise empty
container.

---

Hardening/security design space:

- capabilities - most familiar with this, seems like a good first stab at
  minimizing priviliges
- seccomp - syscall set should be fairly limited and easy to audit.
- SELinux - seems to be able to do everything capabilities and fsGroup can but
  with fine grained control through labels for the server process, the network
  interface and the storage directory. need to study more.
- AppArmor - not familiar at all. k8s support still in beta. seems to be
  somewhere in between seccomp and selinux in terms of conceptual model? i think
  i prefer selinux's relational approach through labels. need to study this more
  as well.



## More code, on disk layout design iterations

Sat evening-late night
Sun afternoon

Summary (in hindsight): Iterated through code, ended up redesigning previous
attempt at persistent storage, motivated by learning about rolling deployments
in kubernetes. Retained most of the code from the abandoned PR.

### MVP, Incremental improvements

Started implementing my line of thinking over the last few days, to first meet
the specification requirements and "ship" a minimally viable version, as an
excuse to learn more about k8s production considerations.

#### Rolling Deployments

The MVP (including `query`) would be a slow start and only signal readyness upon
completion. Since this is currently a single instance stateful application, this
some downtime is necessary during deployments.

By implementing graceful shutdown and lockfiles, a `ReadWriteShared` volume
(e.g. cephfs) would allow for rolling deployments (instead of recreate) with
technically no downtime: the server can signal readyness and begin accepting
requests potentially before the previous instance has fully shutdown and
released the lock allowing the new instance to start scanning the directory.

However, availability would sitll be degraded, as all requests must wait until
the state has been restored, with the same total downtime. If I'm not mistaken
this provides no improvement over the k8s loadbalancer.

#### Reducing Downtime

Although in the worst case (all clients want the last known price a product that
was only used in the first reprice request) read availability downtime would
still be the same, for non pathological cases reads can be satisfied as data is
available, instead of using `Load` on the `sync.Map` the `LastPrice` method
could call `LoadOrStore(productKey, make(chan chan entry))`, and if no entry was
present, block for the result by writing a new `chan entry` to the stored
channel, and then reading the `entry` struct from it. 

When the linear scan gets back such a channel from `LoadOrStore` it signifies a
missing `productId` so the channel can can just be overwritten with the price
data from the scan, and a new goroutine started to range over the channel,
writing the new price data into each `chan entry` it reads from it.

When the linear scan has concluded any remaining channels in the map can simply
be closed, to indicate that a `productId` is not found in the database.

#### Reducing Random Access Complexity (still O(N))

By adding a []unint64 (up to 80 bytes) serialized as base32 (up 128 bytes, well
within portable 256 byte filename length limit) containing hashes of the
productId sorted by count.

The idea behind this was to be able to better support random reads of
`productId`s eventually completely the slow start approach, with immediate write
availability on startup (but with increased complexity due to `previousPrice`,
though still conforming to 202 status code semantics) with degraded read
availability (last price query upper bound is still O(N)).

Since not all last known prices will necessarily be available at `reprice` time,
this reprice requests must first be written to a staging area, and whenever a
mising `previousPrice` property is obtained, a background task can update the on
disk file to add it. An explicitl value of `null` indicates a `productId`'s
first entry in the database. When all objects contain a `previousPrice` property
a file is considered finalized, and moved into the `results` directory.

For random reads of the last price during this initial phase when the last
known prices map is incomplete, a missing `productId` initiates a scan the file
list backwards for matching hashes, with the corresponding file loaded to search
for the last price of the given productId. Hash collisions are tolerable, as the
search can just be resumed after any false positives.

Unfortunately this means two last known price sets must be maintained, one for
the last known prices up to server start, and one for prices updated since then.

When the full set has been loaded the new prices can just be merged into the
main table, from which point the starting assumptions of the simpler slow start
implementation are satisfied, and all last known prices can be served from main
memory.

#### Concurrency Optimizations of Scan and Random Reads

If a match is found in the the last file as of server start, it can be
opportunistically scanned backwards in its entirety, since all `reprice` writes
to the map are serialized the last `newPrice` of a `productId` not in the map
would be up to date.

Since this is a co-recursive process, by induction (or is it co-induction?) the
last file condition can be extended the most recent file that hasn't been fully
scanned. In other words random reads can be done fully concurrently with the
already implemented reverse linear scan at startup, but readyness can be
signalled immediately instead of waiting for it to conclude.

This monotonicity of the last known prices also applies to the start point for
scanning of hashes. Furthermore, random reads that terminate earlier than this
latest unscanned file can still be opportunistically used for other productIds,
by maintaining a bloom filter of the hashes occurring after them (seen earlier
in the backwards scan), and only using the `newPrice` value for `productId`s not
in the filter.

These filters could also be used to share work between hash scans in
since new requests whose `productId` is not in a filter can skip ahead at least
that far, and false positives are a non issue because the filters are used to
confirm absence of keys.

#### Sub Linear Memory Requirements

Finally, if the requirement to keep all productIds in memory is
relaxed, a lazily built bloom filter tree (binary tree where the parent of two
child filters is their sum, where leaf filters correspond to sparse intervals)
could instead be kept instead in order to facilitate efficient random disk reads
on the time series data in the results directory with very low overhead O(log N)

An LRU cache of last known prices layered on top of an an ARC cache of file
contents can be kept for efficiency, most suitably adaptive replacement cache,
to balance the `product` and `query` endpoint workloads.

#### Query Pagination Complexity

The encoded hashes are of course also useful for the `productId` parameter to
the `query` endpoint, including returning a sorted data set the logical
disjunction of multiple such query parameters.

Since all files have a timestamp, the `to` and `from` points can be quickly
found with a binary search on the file list. This slice can then be filtered by
hash.

Pagination of the results is still a bit tricky because there are errors in both direction.

Hash collisions can cause overestimates of the data result set size, but even 96
bit hashes would be possible with 192 bytes of base32 in the filename.

This leaves understimation, which can be made more efficient to a certain extent
also recordng the counts of hashes that appear more than once in a given file
due to frequent price updates, but that's still fundamentally complexity linear
in the page number.

File lists for individual product IDs still need to be fully loaded to handle
queries, but this can be done by keeping an ARC cache of file lists to cache
these data.

#### Sublinear Pagination Requires Aliasing

In order to to paginate in sub-linear time, the list of files pertaining to a
`productId` needs to be efficiently searchable by offsets.

Since files can contain price updates for multiple products, it's not possible
to split the files into disjoint subsets keyed by `productId`. Since there is a
potentially arbitrary number of distinct product IDs, keeping any indices in
memory is potentially prohibitively expensive.

#### Aliasing On Disk

An obvious solution is to create an index subdirectory, with one subdirectory
per product ID (the subdirectory name should be a cryptographic hash since there
are no length constraints on product IDs) and within these subdirectories add
symbolic links to the actual files from the shared global sequence, with the
original naming format (no hashes), but with the `firstEntry` field
corresponding to that `productId`'s offsets.

This would allow mapping offsets within a specific product's sequence of price
updates to files in O(log N), where N is the number of updates for that
particular product.

This approach additional advantage that all last known prices, including non
existent `productId` can be handled in sub=linear time without requiring the
full set of `productIds` to be available in memory, allowing `previousPrice`
data points to be handled with no background thread 

#### Degraded Consistency

Using the filesystem to index products has deisrable scaling properties, but has
weaker consistency guarantees than atomic renaming. Before the server signals
readyness a consistency check (or repair) must be done.

Although writes to the global results directory are atomic and serialized,
creating new symbolic links in the index directories might lag, which could mean
stale price indices if the server is terminated after syncing the file but
before syncing the link.

Being robust to these failures reintroduces a linear cost in the number of total
price updates, since each product directory (of which there may be N) must be
checked for for missing files.

If metadata and data writes are totally ordered with `data=journal` and `dirsync`
mounts option or preferably the corresponding `+j` and `+D` xattrs (need
`CAP_SYS_RESOURCE`) for the files and directory respectively, a consistency scan
(or repair, since adding links is idempotent it's simple) can be terminated if
no `.tmp` files exist, and the latest results file's records are all accounted
for in the indexes.

Using hard links instead of symbolic links, a similar optimization can be made
by comparing the link count of the file with the number of entries as stored in
the canonical filename and terminating if they are equal.

This optimization can then fall back to parsing the file in order to compare the
link count with the number of distinct product IDs, which might be lower. This
can be fixed by adding the number of distinct product IDs in the file name along
with the other metadata.

Better yet, if the same file is hard linked multiple times in a single product
subdirectory, once per instance of a corresponding record with the `firstEntry`
field mapping 1:1, file lists can be pretty directly converted to roaring
bitmaps without sorting in advance, which has some nice advantages for querying,
especially if `productId` is allowed to be specified multiple times to compute a
union.

#### Cleaning Up .tmp Files

Temporary files and symbolic links to them could simply be discarded due to the
durability guarantees of the REST api.

To facilitate this cleanup, links should be created after syncing but before
renaming tempfiles, so that the links can be removed (symbolic links can be
lazily pruned but hard links need to be preemptively pruned, as the content
may remains even if the `results` directory reverts to some prior state).

#### "Crash Only" Approach Instead of Graceful Shutdown

A crash only software model is preferable to graceful shutdown handlers because
it is more robust. This is appropraite, since writes require only 202 type
consistency, and other endpoints are readonly, there is no technical requirement
for finishing in flight requests, and something like a service mesh can handle
reattempts for the GET requests and unaccepted (e.g. 503) `reprice` requests.

#### Improved Durability for "Crash Only" by eliminating tempfiles

Instead of appending new entries to a slice which is then written to disk on
flush, files could be initialized with a `[\n`, followed by unbuffered JSON
encoding delimited by `,\n\t` (apart from the last entry, which is terminated
by a `]` and synced to disk).

If the last character of the last file is a `]` and it can be parsed
successfully then the database is consistent. If the last character is not a
closing bracket, one can be added using `io.MultiReader` before attempting to
parse.

Given the lack of length constraints the data is not guaranteed but is still
likely to fit in a single block, but partially written records must still be
handled. If the file is still invalid, it is due to the last record being only
partly written, and the file can be truncated after the last balanced closing
`}` terminating a record.

If the current time is still within the time window based on the filename, it
can be appended instead of a new file created.

#### NDJSON Result Files

Although the specification implied files must be indented multiline JSON,
newline delimited JSON is much simpler to recover from partially written records
within a file - simply truncate to the last newline in the file, no need to
attempt to parse or any of th `io.MultiReader` complexity.

## Revised MVP

Mon afternoon

Working backwards, the simplest MVP (including `query`) meeting the
specification requirements would therefore be a fast start single instance
stateful application, that has a lazily populated in memory cache of last known
prices.

To avoid deviating from the specification, NDJSON will not be used, foregoing
complex recovery through parsing to retain the same simplicity.

#### Storage Layout

##### Directory Tree Structure

- `.../repricer/`
  - `results/` - global sequence of entries broken up into files
    - `{fileSeq}-{entrySeq}-{nRecords}-{distinctProductIds}-{unixSec}-{nanosec}.json`
  - `results_by_product/`
    - `{H(productId)}` - where H is e.g. sha256, because productId length unconstrained
      - `{fileSeq}-{entrySeq}-{nRecords}-{distinctProductIds}-{unixSec}-{nanosec}.json` (as hard links)

##### Fields Embedded In Filename

`fileSeq` field should be encoded fixed width format for lexicographical
sorting.

all fields could be encoded as hex for efficient conversion and fixed width format?

- `fileSeq` - global sequence number for file
- `entrySeq` - sequence # in that directory, in `results`, global sequence number of first entry in file, in
  `results_by_product` per product sequence number of first entry
- `nRecords` - in `results`, number of records in file, in `results_by_product`
  it's the number of records with for that `productId` in the file.
- `distinctProductIds` - number of distinct `productId`s.
- `unixSec`, `nanoSec` - values of nominal timestamp
  
##### Invariants of Consistent Storage Directory

- for all directories:
  - `fileseq_{n+1} > fileSeq_{n}` - may be sparse in product product directories
  - `entrySeq_{n+1} == entrySeq_{n} + nRecord_{n}`
  - `unixSec_{n+1} >= unixSec_{n}`
  - `(unixSec_{n+1} * 1e9 + nanoSec_{n+1}) > ( unixSec_{n} * 1e9 + nanoSec_{n} )`
    (theoretically `>=`, but empirically `t0, t1 := Now(), Now();
    t1.Sub(t0).Nanoseconds` is of order 1e3
- for global results directory:
  - `fileseq_{n+1} == fileSeq_{n} + 1`
- for all files in all directories:
  - `syscall.Stat_t.Nlink == distinctProductIds + 1`
  - `nRecords` equals number of records in the root array (or `\n`s if using
    ndjson).
    
    Note: if records are written into files individually, nRecords and
    distinctProductIds invariants must be invariant will not hold before a batch
    has finished writing.
- for all links/filenames grouped by inode:
   - `fileSeq` is the same in all directories
   - `nRecords_{results} == sum( nRecords_{product_0..product_n} )`
- for all products 
  - all but first record have previousPrice set, and equal
  - all items have monotonicly increasing timestamps

make part of public API:

quick scan & repair - for use in startup
full scan - should be safe to do in parallel for use in integration tests 


##### Consistency and Durability

Goroutines of requests handled by the HTTP handlers call a concurrency safe
model API to store and retrieve prices.

The model linearizes these calls into a total order, assigns timestamps (assumed
monotonic, even though monotonic clock values are *not* used) and a sequence
numbers, looks up the last known prices, and writes to storage.

Write operations depend on a random read. Since random reads are have sub-linear
complexity in this layout (counting readdir as O(1)), writes are fully processed
on-line before being written into storage in the linearized order.

`previousPrice` fields need consistent values, whereas `price` endpoint is not
specified (will implement inconsistent reads)

#### Internal State Machine of Storage Directory

1. unlocked - wait to obtain exclusive lock
2. locked, unvalidated - while most recent file's `syscall.Stat_t.Nlink < distinctProductIds + 1` in global dir, try to parse
   - if valid, add any missing links by re-indexing `productId`s 
     known price index, add all entries to in memory cache, and continue to next
     most recent file
   - *omitted for simplicity over durability. ndjson is best of both worlds*:
     - if valid after appending `]` with `io.MultiReader`, add missing links.
     - if timestamp is within flush deadline, open last file as current output file.
   - if invalid, unlink back references from `productId` and finally `results`
     dir entry when link count == 1.
3. degraded availability (readyness can be signalled)
4. full availability - in memory database is full when map size == # productId
   directories

Exact complexity of the loops depending on whether or not dirsync is enabled.
With dirsync there should be at most 1 partially indexed file. Are there
meaningful constant bounds without it?

#### Desirable Properties

- Fast Start: write and read availability immediately after obtaining lock
- Cold start write availability does not require a background task or complex
  state machine (e.g. pending directory, tempfiles) for directory structure,
  even after crash.
- New data gets written to file descriptor reasonably early, especially with
  warm in memory data set (it could be optimal if `previousPrice` is removed,
  but with it it's a trade off between durability and disk churn since every
  write has a data dependency on a random read)
- Query page offsets can be resolved in sub-linear time, with same code path for
  no filtering or single `productId` filter with good support for
  rich queries in the future (i.e. new fields/dimensions, using roaring bitmaps
  or something similar to compute sparse maps of logically rich queries over
  high dimensionality space)
- Apart from partially written records on disk state is monotone.
- Partially written records could be categorically eliminated simply by
  (generously) constraining the length of `productId` and `price` fields, in
  order to guarantee atomicity of record writes.
- administrable using kubernetes security context:
  - `runAsUser`, `runAsGroup`
  - `fsGroup`
- compatible with `xattrs` or `ioctl_iflags`
  - durability: `+S` or just `O_SYNC` - provides durability (but 202 code)
  - consistency: `+jDS` - data journaling, dirsync, syncing of all writes in combination 
    provide consistent reads 
  - data protection `+ia`
  - tuning, administration: `+cdPA`
- If the in memory data set requirement is relaxed, `syscall.Mmap` can make file
  data available as a `[]byte` and parsed with e.g.
  [`simdjson-go`](https://github.com/fwessels/simdjson-go), and accessing the
  mmapped regions through cached tapes objects.
  This also delegates management of much of the working memory set almost
  entirely to the kernel. This approach should work better with larger numbers of
  records per file, striking a balance between mmap overheads vs. SIMD parsing
  overhead for facilitating random access.

#### Downsides

- indented JSON is a suboptimal format, both compared to NDJSON, and compared
  to binary formats, especially for non human readable data.
- design space of a relational/graph databases, distributed or otherwise, is
  clearer/simpler than so called "plain files" on disk, given the complexity of
  Linux's or even just POSIX's notion of filesystems.
- In particular directory scans provide no ordering guarantees, so given the
  constraints that imply cardinality of files is practically O(N), there will
  always be some overhead, both ram and computation, to load/scan/search these
  structures.
  This could be mitigated by chunking up the storage directory
  into non overlapping or partly overlapping subdirectories, totally ordered
  numbered subdirectories such that `∀A,B`, two subdirectories, `A<B` in the
  total order relation iff `∀a∈A` and `∀b∈B`: `a<b` in the partial order
  relation over events.

## Final Push

Wed-Thu

This digression about deployment and storage consistency has been very
instructive, but has gone too far, trying to wrap things up, Hoefstater's Law
struck as always.

Made a few architectural mistakes that necessitated some ugly workarounds in the
interest of expediency.

I also had to cheat and write more unit tests than I intended (as per
requirements, I selected the linearizer as the *only* component to unit test).
However, I needed them in the aid of development/debugging and it felt silly to
delete after the fact. Perhaps I can get off on a technicality, in that
technically all unit tests are in the storage package ;-)

---

https://github.com/kubernetes/kubernetes/issues/58512

---

I think attaching another pod won't work since it's a ReadWriteOnce volume.

---

Investigating ephemeral containers.

Oh, it appears it's just a SIG proposal.

---

I'm reasonably confident the reason for the startup crash is that the physical
volume has data from the previous version.

---

Heh, just had a bit of a fight with the deployment resource, trying to delete
pods & pvs, and not understanding why they won't go away... oops.

## Relase

Fri

After spotting a minor regression from yesterday (which really should have been
caught by proper unit tests, had those existed) I merged all the pending PRs.

---

Kind of confused about release pipelines, and how they relate to AKS/ACR. I'm
not seeing ACR in the artifacts when trying to set up a new release pipeline.

The documentation on azure devops for deploying to k8s seems lacking:
- ["Classic" mode instructions](https://docs.microsoft.com/en-us/azure/devops/pipelines/apps/cd/deploy-aks?view=azure-devops&tabs=java)
  seem to have more instructions that partly overlap with stuff that's already been done:
- [recent
  instructions](https://docs.microsoft.com/en-us/azure/devops/pipelines/ecosystems/kubernetes/aks-template?view=azure-devops)
  seem to end where I already am.
  
Does this mean it's already deployed in a stable environment? I was under the
impression that the deployment based on the master branch is more of a staging
environment, and that the service IPs were not necessarily stable, but it
appears that's not the case:

```
kubectl get services
NAME         TYPE           CLUSTER-IP     EXTERNAL-IP    PORT(S)        AGE
kubernetes   ClusterIP      10.0.0.1       <none>         443/TCP        8d
repricer     LoadBalancer   10.0.183.210   51.137.6.251   80:31126/TCP   8d
```

I will therefore consider this "deployed to production" in some sense. Now I
just need need to finish `README.md` and I'm done.

---

That's a wrap.
