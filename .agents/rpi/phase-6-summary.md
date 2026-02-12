# Post-mortem: Phase 2 — Sensor Simulator

## What went well
- Clean implementation following established Phase 1 patterns
- gRPC client usage matched test patterns exactly
- All 21 tests pass including integration test with real gRPC server
- Race detector clean
- Dead-reckoning math validated with directional unit tests

## Learnings
- `anypb.New()` requires protobuf message types registered via buf-generated code — worked seamlessly
- `grpc.NewClient()` is the current API (not the deprecated `grpc.Dial()`)
- Integration test pattern (start real server, run simulator, verify state) is robust and fast (~600ms)

## Next steps (Phase 3: Classifier)
- Watch stream consumer that reads Track updates
- Classify tracks based on speed/heading patterns
- Add ClassificationComponent and ThreatComponent to entities
