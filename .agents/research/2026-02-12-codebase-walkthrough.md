# Lattice Lab: A Guided Codebase Tour

**Reading this like a K8s operator would:** This system is a mini version of a distributed C2 (command & control) data lattice. If K8s is "orchestrate containers," this is "orchestrate entities" — tracks, assets, and geographic points flowing through a pipeline of watchers that react to changes, exactly like K8s controllers reacting to resource events.

---

## Layer 0: The Data Model (proto files)

**Read this first.** Everything else is plumbing around these shapes.

```
proto/entity/v1/entity.proto
─────────────────────────────

// Think: CustomResourceDefinition. This defines what "things" exist in the system.

Entity {
    id          string              // like metadata.name in K8s
    type        EntityType          // like kind: (Track | Asset | Geo)
    components  map<string, Any>    // like spec: — a bag of typed sub-objects
    created_at  Timestamp           // like creationTimestamp
    updated_at  Timestamp           // like last-applied-configuration time
}

// "components" is the ECS pattern (Entity-Component-System).
// Instead of inheritance, entities are bags of mix-and-match data.
// A Track might have: {position, velocity, classification, threat, task_catalog}
// Think: labels + annotations, but typed and structured.

EntityType = UNSPECIFIED | ASSET | TRACK | GEO

ThreatLevel = UNSPECIFIED | NONE | LOW | MEDIUM | HIGH

// The components themselves — each is just a simple struct:
PositionComponent   { lat, lon, alt }           // Where is it?
VelocityComponent   { speed, heading }          // How fast, which direction?
ClassificationComponent { label, confidence }   // What do we think it is?
ThreatComponent     { level }                   // How dangerous?
TaskCatalogComponent { available_tasks[] }       // What should we do about it?
```

```
proto/store/v1/store.proto
──────────────────────────

// Think: the K8s API server. This is the CRUD + Watch interface.

service EntityStoreService {
    CreateEntity(entity)   → Entity          // like kubectl apply (new)
    GetEntity(id)          → Entity          // like kubectl get
    ListEntities(filter?)  → Entity[]        // like kubectl get --all
    UpdateEntity(entity)   → Entity          // like kubectl apply (existing)
    DeleteEntity(id)       → Empty           // like kubectl delete
    WatchEntities(filter?) → stream Event    // like kubectl get --watch
    //                        ^^^^^^
    // THIS is the key. Server-streaming RPC.
    // Returns a never-ending stream of {CREATED | UPDATED | DELETED, entity}
    // Just like K8s informers watching for resource changes.
}

EventType = CREATED | UPDATED | DELETED
EntityEvent { type, entity }
```

**Key insight:** The `Watch` RPC is the backbone of this whole system. Every downstream service subscribes to entity changes through it. This is the same pattern as K8s informers — you don't poll, you watch.

---

## Layer 1: The Store (the "etcd" of this system)

```
internal/store/store.go — The in-memory database
─────────────────────────────────────────────────

// K8s analogy: etcd + the apiserver's storage layer, merged into one.
// This is a thread-safe map with a built-in pub/sub system.

Store {
    entities  map[id → Entity]     // the "database" (like etcd's key-value store)
    ttls      map[id → expiry]     // optional auto-cleanup (like K8s garbage collection)
    watchers  []*Watcher           // subscribers (like K8s informers)
    mu        RWMutex              // reader-writer lock (many readers OR one writer)
    watchMu   RWMutex              // separate lock for the watcher list
}

Watcher {
    Filter  EntityType             // "only tell me about Tracks" (like a label selector)
    Events  channel of EntityEvent // Go channel = async mailbox, buffered to 64
}


// ── CRUD operations ──

Create(entity):
    LOCK the store
    if entity.id already in map → error "already exists"
    clone the entity (defensive copy — caller can't mutate our data)
    stamp created_at and updated_at = now
    store it
    UNLOCK
    NOTIFY all watchers: {CREATED, entity}
    return a clone (again, defensive — we keep our copy safe)

Get(id):
    READ-LOCK (multiple readers allowed simultaneously)
    look up in map → return clone, or error "not found"

List(typeFilter):
    READ-LOCK
    for each entity in map:
        if filter is set and entity.type doesn't match → skip
        collect clone into results
    return results

Update(entity):
    LOCK
    if entity.id not in map → error "not found"
    clone incoming, preserve original created_at, stamp new updated_at
    replace in map
    UNLOCK
    NOTIFY all watchers: {UPDATED, entity}

Delete(id):
    LOCK
    if id not in map → error "not found"
    remove from map
    UNLOCK
    NOTIFY all watchers: {DELETED, entity}


// ── Watch system (the informer pattern) ──

Watch(typeFilter) → *Watcher:
    create a Watcher with a buffered channel (64 slots)
    add it to the watchers list
    return it
    // Caller reads from watcher.Events channel to get live updates

Unwatch(watcher):
    remove from list, close the channel

notify(event):   // called internally by Create/Update/Delete
    READ-LOCK the watcher list
    for each watcher:
        if watcher has a type filter and event doesn't match → skip
        try to send event into watcher's channel
        if channel is full → DROP the event (non-blocking)
        // This prevents a slow consumer from blocking the whole store.
        // K8s does the same: informers that fall behind get re-synced.


// ── TTL / Reaper (garbage collection) ──

SetTTL(id, duration):
    record "this entity expires at now + duration"

StartReaper(ctx, interval):
    every <interval>:
        scan the ttl map
        collect IDs where expiry < now
        for each expired ID: call Delete(id)
    // Runs until context is cancelled.
    // Like K8s TTL controller for finished Jobs.
```

---

## Layer 2: The gRPC Server (the "API server")

```
internal/server/grpc.go — Thin adapter: gRPC → Store
─────────────────────────────────────────────────────

// K8s analogy: the kube-apiserver handler layer.
// This file is intentionally boring. It just translates gRPC requests
// into Store method calls and maps errors to gRPC status codes.

Server {
    store  *Store
    // also embeds UnimplementedEntityStoreServiceServer
    // (required by gRPC to be forward-compatible with new RPCs)
}

CreateEntity(request):
    validate: entity must exist, id must be non-empty
    call store.Create(entity)
    on error → gRPC AlreadyExists
    return entity

GetEntity(request):
    call store.Get(id)
    on error → gRPC NotFound

ListEntities(request):
    call store.List(typeFilter)
    return {entities: results}

UpdateEntity(request):
    validate: entity must exist
    call store.Update(entity)
    on error → gRPC NotFound

DeleteEntity(request):
    call store.Delete(id)
    on error → gRPC NotFound
    return Empty

WatchEntities(request, stream):
    // THIS is the interesting one.
    register a watcher with the store (filtered by type)
    defer: unwatch when stream closes

    forever:
        select:
            case event from watcher channel:
                if channel closed → return (watcher removed)
                send event over the gRPC stream
            case stream context done:
                return (client disconnected)

    // The client calls WatchEntities once and gets a never-ending
    // stream of events. Just like `kubectl get pods --watch`.
```

---

## Layer 3: The Binaries (the "Deployments")

Each `cmd/` directory is a standalone binary. Think of them as separate K8s Deployments — they each run independently and talk to each other over gRPC.

```
cmd/entity-store/main.go — The API Server
──────────────────────────────────────────

// K8s analogy: deploying kube-apiserver

main():
    port = env PORT or "50051"
    listen on TCP :{port}

    create Store (the in-memory database)
    create gRPC Server, register the EntityStoreService handler
    enable reflection (so grpcurl can discover the API — like kubectl api-resources)

    spawn goroutine:
        wait for SIGINT or SIGTERM
        call GracefulStop (finish in-flight RPCs, then stop)
        // Like a K8s preStop hook — drain connections before dying

    serve until stopped
```

```
cmd/sensor-sim/main.go — The Data Source
────────────────────────────────────────

// K8s analogy: a CronJob or DaemonSet that generates metrics/events

main():
    load config from defaults
    override with env vars: STORE_ADDR, INTERVAL, NUM_TRACKS, BBOX_*
    // Standard 12-factor config pattern (like K8s env vars in a Deployment spec)

    create cancellable context
    spawn signal handler goroutine (SIGINT/SIGTERM → cancel context)

    create Simulator with config
    run until context cancelled
```

```
cmd/classifier/main.go — A Controller
──────────────────────────────────────

// K8s analogy: a controller that watches Pods and adds annotations

main():
    load config, override STORE_ADDR from env
    create cancellable context + signal handler
    create Classifier, run until cancelled

    // Same boilerplate pattern as sensor-sim.
    // Every service: defaults → env override → context → signal → run
```

```
cmd/task-manager/main.go — Another Controller
──────────────────────────────────────────────

// Identical pattern. Load config, run until killed.
// Like a K8s operator watching for CRD changes.
```

---

## Layer 4: The Controllers (the "operators")

This is where the interesting logic lives. Each one follows the **controller pattern**: watch for changes, react, write back.

```
internal/sensor/simulator.go — The Track Generator
───────────────────────────────────────────────────

// K8s analogy: an init container or sidecar that produces data
// Real-world analogy: a radar that detects aircraft and reports positions

Config {
    StoreAddr   "localhost:50051"    // where to send data
    Interval    1 second             // how often to update
    NumTracks   5                    // how many aircraft to simulate
    BBox        {lat/lon bounds}     // geographic area (defaults to DC metro)
}

track {
    id       "track-0", "track-1", etc.
    lat, lon, alt         // current position
    speed                 // meters per second
    heading               // compass direction (0°=north, 90°=east)
    created  bool         // have we told the store about this track yet?
}

Simulator {
    config
    tracks[]              // the simulated aircraft
}

New(config) → Simulator:
    for i in 0..NumTracks:
        spawn a random track within the bounding box
        random altitude 1000-6000m
        random speed 100-500 knots (converted to m/s)
        random heading 0-360°

Run(ctx):
    connect to entity-store via gRPC
    start a ticker (fires every Interval)

    loop:
        wait for tick (or context cancelled)
        for each track:
            tick(track)

tick(track):
    if track not yet created in store:
        build an Entity with position + velocity components
        call CreateEntity on the store
        mark track as created
    else:
        advance the track's position (dead-reckoning)
        build updated Entity
        call UpdateEntity on the store

advanceTrack(track, timeDelta):
    // Dead reckoning: "I know where I was, how fast I'm going, and which
    // direction — so I can estimate where I am now."
    // Like a K8s HPA projecting future load from current trend.

    convert heading to radians
    distance = speed × time

    Δlat = distance × cos(heading) / 111320     // meters per degree latitude
    Δlon = distance × sin(heading) / (111320 × cos(lat))  // adjusted for longitude shrinkage

    // This is a flat-earth approximation. Fine for small areas like DC metro.

buildEntity(track) → Entity:
    pack PositionComponent {lat, lon, alt} into protobuf Any
    pack VelocityComponent {speed_in_knots, heading} into protobuf Any
    // anypb.New() is like json.Marshal — it wraps a typed struct in a
    // generic envelope so the store can hold any component type.
    return Entity{id, type=TRACK, components={"position": pos, "velocity": vel}}
```

```
internal/classifier/classifier.go — The "Admission Webhook"
────────────────────────────────────────────────────────────

// K8s analogy: a mutating admission webhook. It watches resources come in
// and enriches them with additional data (adds labels/annotations).
// Here: watches Tracks, adds classification + threat components.

Classify(speed_in_knots) → Classification:
    // Simple rule engine. Like a K8s NetworkPolicy deciding allow/deny.

    if speed < 150 kts  → "civilian",  confidence 85%, threat NONE
    if speed 150-350    → "aircraft",  confidence 70%, threat LOW
    if speed > 350      → "military",  confidence 90%, threat HIGH

Classifier {
    config (just StoreAddr)
}

Run(ctx):
    connect to entity-store
    call WatchEntities(filter=TRACK)    // "subscribe to all Track events"
    // Like: kubectl get tracks --watch

    loop forever:
        receive next event from stream
        if event is DELETE → skip (nothing to classify)
        classifyEntity(event.entity)

classifyEntity(entity):
    extract speed from entity.components["velocity"]
    // Unpack the Any → VelocityComponent → read .Speed
    // Like: kubectl get track -o jsonpath='{.spec.velocity.speed}'

    run Classify(speed) → get label, confidence, threat

    pack ClassificationComponent{label, confidence} → Any
    pack ThreatComponent{level} → Any

    attach both to entity.components["classification"] and ["threat"]
    // Mutating the entity's "spec" — adding new fields

    call UpdateEntity → write enriched entity back to store
    // Now the entity has 4 components: position, velocity, classification, threat
    // The store fires an UPDATED event, which other watchers will see.
```

```
internal/task/manager.go — The "Operator" (state machine)
──────────────────────────────────────────────────────────

// K8s analogy: a full operator with a reconcile loop.
// It watches for threat levels and decides what actions to take.
// Like an operator that watches a CRD and manages child resources.

State = "idle" | "investigate" | "track" | "intercept"
// Think: Pod phases (Pending, Running, Succeeded, Failed)

Assignment {
    EntityID   string
    State      State       // current operational state
    Tasks      []string    // what actions to perform
}

Rules(threat) → (State, Tasks):
    // The "desired state" calculator. Given a threat level, what SHOULD we be doing?

    NONE   → idle,        no tasks
    LOW    → investigate,  [monitor, identify]
    MEDIUM → track,        [monitor, identify, track]
    HIGH   → intercept,    [monitor, identify, track, intercept]

    // Each higher threat level is a superset of the lower ones.
    // Like K8s: a Deployment with more replicas has all the pods of fewer replicas + more.

Manager {
    config
    assignments  map[entityID → Assignment]   // current state (like status subresource)
    mu           RWMutex
}

Run(ctx):
    connect to entity-store
    call WatchEntities(filter=TRACK)

    loop forever:
        receive event
        if DELETE → removeAssignment(entityID)    // cleanup, like finalizer
        else     → processEntity(entity)

processEntity(entity):
    extract threat level from entity.components["threat"]
    if no threat component → skip (classifier hasn't run yet)
    // Like: "don't reconcile until the dependency is ready"

    compute desired state: Rules(threat) → state, tasks

    compare to current assignment:
        if same state → no-op (already reconciled)
        if changed → update local assignment map

    if tasks changed:
        pack TaskCatalogComponent{tasks} → Any
        attach to entity.components["task_catalog"]
        call UpdateEntity
        // Now entity has 5 components: position, velocity, classification, threat, task_catalog

removeAssignment(entityID):
    delete from local assignments map
```

---

## Layer 5: The Mesh Relay (multi-cluster replication)

```
internal/mesh/relay.go — The "Federation" Layer
────────────────────────────────────────────────

// K8s analogy: KubeFed or Submariner — replicating resources between clusters.
// This watches one entity-store and mirrors everything to peer stores.

Config {
    LocalAddr  "localhost:50051"    // the "home cluster"
    Peers      []string             // other clusters to sync to
}

Relay {
    config
    stats  {Forwarded, Errors}     // counters for observability
}

Run(ctx):
    if no peers → error (nothing to replicate to)

    connect to local store
    connect to ALL peer stores

    call WatchEntities on local store (no filter — watch EVERYTHING)

    loop forever:
        receive event from local store
        forwardToPeers(event)

forwardToPeers(event):
    for each peer:
        forwardEvent(peer, event)
        on success → stats.Forwarded++
        on error   → stats.Errors++, log it, keep going
        // Best-effort. One peer failing doesn't stop others.

forwardEvent(peer, event):
    // The magic is in the error handling — it's idempotent replication.

    switch event.type:
        CREATED:
            try CreateEntity on peer
            if AlreadyExists → fall back to UpdateEntity
            // Peer might already have it (from a previous sync or another relay)
            // Like: kubectl apply — create if new, update if exists

        UPDATED:
            try UpdateEntity on peer
            if NotFound → fall back to CreateEntity
            // Peer might not have it yet (joined late, or missed the create)
            // Like: kubectl apply on a resource that doesn't exist yet

        DELETED:
            try DeleteEntity on peer
            if NotFound → that's fine, ignore
            // Already gone on peer. Idempotent delete.
            // Like: kubectl delete on something that's already deleted → no error
```

---

## Layer 6: The CLI (the "kubectl")

```
cmd/lattice-cli/main.go — Operator Interface
─────────────────────────────────────────────

// K8s analogy: kubectl. Uses Cobra (same CLI framework as kubectl itself).

Globals:
    storeAddr  flag: --store (default "localhost:50051")

dial() → (client, cleanup, error):
    // Helper: connect to entity-store, return client + cleanup function.
    // Pattern: caller does `defer cleanup()` — ensures connection closes.

Commands:

    list [--type track|asset|geo]:
        connect to store
        call ListEntities with optional type filter
        print table: ID | TYPE | COMPONENTS | UPDATED
        // Like: kubectl get pods

    get <id>:
        connect to store
        call GetEntity
        print: ID, Type, Created, Updated, component names
        // Like: kubectl describe pod <name>

    watch:
        connect to store
        call WatchEntities (filter=TRACK)
        print each event as it arrives: [CREATED] track-0  components=position,velocity
        // Like: kubectl get pods --watch
```

---

## The Big Picture: Data Flow

```
                    ┌─────────────┐
                    │  sensor-sim  │  "the radar"
                    └──────┬──────┘
                           │ Create/Update (position + velocity)
                           ▼
                    ┌─────────────┐
                    │ entity-store │  "etcd + apiserver"
                    └──────┬──────┘
                           │ Watch stream (TRACK events)
                    ┌──────┴──────┐
                    ▼             ▼
             ┌────────────┐ ┌────────────┐
             │ classifier  │ │task-manager│
             │ "webhook"   │ │ "operator" │
             └──────┬──────┘ └─────┬──────┘
                    │              │
                    │ Update       │ Update
                    │ (+class,     │ (+task_catalog)
                    │  +threat)    │
                    ▼              ▼
                    ┌─────────────┐
                    │ entity-store │  (same store, enriched entities)
                    └──────┬──────┘
                           │
              ┌────────────┼────────────┐
              ▼            ▼            ▼
        ┌──────────┐ ┌──────────┐ ┌──────────┐
        │mesh-relay│ │lattice-cli│ │  (more   │
        │"kubefed" │ │ "kubectl" │ │watchers) │
        └──────────┘ └──────────┘ └──────────┘
```

**The controller loop in 4 sentences:**

1. **Sensor-sim** creates tracks with `{position, velocity}` — raw sensor data.
2. **Classifier** watches those tracks, reads speed, adds `{classification, threat}` — enriched data.
3. **Task-manager** watches enriched tracks, reads threat, adds `{task_catalog}` — actionable decisions.
4. **Mesh-relay** watches everything and copies it to peer stores — replication.

Each controller only writes what it knows. The entity accumulates components over time like a snowball. This is the ECS (Entity-Component-System) pattern — the same architecture used in game engines and, conceptually, in K8s itself (where a Pod accumulates status conditions from different controllers).
