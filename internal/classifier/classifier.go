package classifier

import (
	"context"
	"fmt"
	"log/slog"

	entityv1 "github.com/boshu2/lattice-lab/gen/entity/v1"
	storev1 "github.com/boshu2/lattice-lab/gen/store/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/anypb"
)

// Config controls the classifier service.
type Config struct {
	StoreAddr string
}

// DefaultConfig returns classifier defaults.
func DefaultConfig() Config {
	return Config{StoreAddr: "localhost:50051"}
}

// Classification holds the result of classifying a track.
type Classification struct {
	Label      string
	Confidence float32
	Threat     entityv1.ThreatLevel
}

// Classify returns a classification based on speed (in knots).
func Classify(speedKnots float64) Classification {
	switch {
	case speedKnots < 150:
		return Classification{
			Label:      "civilian",
			Confidence: 0.85,
			Threat:     entityv1.ThreatLevel_THREAT_LEVEL_NONE,
		}
	case speedKnots <= 350:
		return Classification{
			Label:      "aircraft",
			Confidence: 0.70,
			Threat:     entityv1.ThreatLevel_THREAT_LEVEL_LOW,
		}
	default:
		return Classification{
			Label:      "military",
			Confidence: 0.90,
			Threat:     entityv1.ThreatLevel_THREAT_LEVEL_HIGH,
		}
	}
}

// Classifier watches Track entities and adds classification + threat components.
type Classifier struct {
	cfg Config
}

// New creates a classifier with the given config.
func New(cfg Config) *Classifier {
	return &Classifier{cfg: cfg}
}

// Run connects to the store, watches Tracks, and classifies them until ctx is cancelled.
func (c *Classifier) Run(ctx context.Context) error {
	conn, err := grpc.NewClient(c.cfg.StoreAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connect to store: %w", err)
	}
	defer conn.Close()

	client := storev1.NewEntityStoreServiceClient(conn)

	stream, err := client.WatchEntities(ctx, &storev1.WatchEntitiesRequest{
		TypeFilter: entityv1.EntityType_ENTITY_TYPE_TRACK,
	})
	if err != nil {
		return fmt.Errorf("watch entities: %w", err)
	}

	slog.Info("classifier watching tracks", "store_addr", c.cfg.StoreAddr)

	for {
		event, err := stream.Recv()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("recv: %w", err)
		}

		if event.Type == storev1.EventType_EVENT_TYPE_DELETED {
			continue
		}

		if err := c.classifyEntity(ctx, client, event.Entity); err != nil {
			slog.Error("classify failed", "entity_id", event.Entity.Id, "error", err)
		}
	}
}

func (c *Classifier) classifyEntity(ctx context.Context, client storev1.EntityStoreServiceClient, entity *entityv1.Entity) error {
	speed, err := extractSpeed(entity)
	if err != nil {
		return err
	}

	cl := Classify(speed)

	clComp, err := anypb.New(&entityv1.ClassificationComponent{
		Label:      cl.Label,
		Confidence: cl.Confidence,
	})
	if err != nil {
		return fmt.Errorf("pack classification: %w", err)
	}

	threatComp, err := anypb.New(&entityv1.ThreatComponent{
		Level: cl.Threat,
	})
	if err != nil {
		return fmt.Errorf("pack threat: %w", err)
	}

	entity.Components["classification"] = clComp
	entity.Components["threat"] = threatComp

	if _, err := client.UpdateEntity(ctx, &storev1.UpdateEntityRequest{Entity: entity}); err != nil {
		return fmt.Errorf("update %s: %w", entity.Id, err)
	}

	slog.Info("classified entity", "entity_id", entity.Id, "label", cl.Label, "confidence_pct", cl.Confidence*100, "threat", cl.Threat.String(), "speed_kts", speed)
	return nil
}

func extractSpeed(entity *entityv1.Entity) (float64, error) {
	velAny, ok := entity.Components["velocity"]
	if !ok {
		return 0, fmt.Errorf("no velocity component")
	}

	vel := &entityv1.VelocityComponent{}
	if err := velAny.UnmarshalTo(vel); err != nil {
		return 0, fmt.Errorf("unmarshal velocity: %w", err)
	}

	return vel.Speed, nil
}
