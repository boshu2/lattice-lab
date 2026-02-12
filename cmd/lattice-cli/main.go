package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	entityv1 "github.com/boshu2/lattice-lab/gen/entity/v1"
	storev1 "github.com/boshu2/lattice-lab/gen/store/v1"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var storeAddr string

func main() {
	root := &cobra.Command{
		Use:   "lattice-cli",
		Short: "Operator interface for Lattice Lab",
	}

	root.PersistentFlags().StringVar(&storeAddr, "store", "localhost:50051", "entity-store address")

	root.AddCommand(listCmd(), getCmd(), watchCmd(), approveCmd(), denyCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func dial() (storev1.EntityStoreServiceClient, func(), error) {
	conn, err := grpc.NewClient(storeAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}
	client := storev1.NewEntityStoreServiceClient(conn)
	return client, func() { conn.Close() }, nil
}

func listCmd() *cobra.Command {
	var typeFilter string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List entities",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cleanup, err := dial()
			if err != nil {
				return err
			}
			defer cleanup()

			filter := entityv1.EntityType_ENTITY_TYPE_UNSPECIFIED
			switch typeFilter {
			case "track":
				filter = entityv1.EntityType_ENTITY_TYPE_TRACK
			case "asset":
				filter = entityv1.EntityType_ENTITY_TYPE_ASSET
			case "geo":
				filter = entityv1.EntityType_ENTITY_TYPE_GEO
			}

			resp, err := client.ListEntities(context.Background(), &storev1.ListEntitiesRequest{
				TypeFilter: filter,
			})
			if err != nil {
				return err
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tTYPE\tCOMPONENTS\tUPDATED")
			for _, e := range resp.Entities {
				comps := componentNames(e)
				updated := ""
				if e.UpdatedAt != nil {
					updated = e.UpdatedAt.AsTime().Format("15:04:05")
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Id, e.Type, comps, updated)
			}
			w.Flush()
			return nil
		},
	}

	cmd.Flags().StringVarP(&typeFilter, "type", "t", "", "filter by type (track, asset, geo)")
	return cmd
}

func getCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get entity details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cleanup, err := dial()
			if err != nil {
				return err
			}
			defer cleanup()

			e, err := client.GetEntity(context.Background(), &storev1.GetEntityRequest{Id: args[0]})
			if err != nil {
				return err
			}

			fmt.Printf("ID:      %s\n", e.Id)
			fmt.Printf("Type:    %s\n", e.Type)
			fmt.Printf("Created: %s\n", e.CreatedAt.AsTime().Format("2006-01-02 15:04:05"))
			fmt.Printf("Updated: %s\n", e.UpdatedAt.AsTime().Format("2006-01-02 15:04:05"))
			fmt.Printf("Components:\n")
			for name, comp := range e.Components {
				fmt.Printf("  %s: %s\n", name, comp.TypeUrl)
			}
			return nil
		},
	}
}

func watchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "watch",
		Short: "Watch entity events in real-time",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cleanup, err := dial()
			if err != nil {
				return err
			}
			defer cleanup()

			stream, err := client.WatchEntities(cmd.Context(), &storev1.WatchEntitiesRequest{
				TypeFilter: entityv1.EntityType_ENTITY_TYPE_TRACK,
			})
			if err != nil {
				return err
			}

			fmt.Println("Watching track events (Ctrl+C to stop)...")
			for {
				event, err := stream.Recv()
				if err != nil {
					return err
				}
				comps := componentNames(event.Entity)
				fmt.Printf("[%s] %s  components=%s\n", event.Type, event.Entity.Id, comps)
			}
		},
	}
}

func approveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "approve <entity-id>",
		Short: "Approve a pending intercept action",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cleanup, err := dial()
			if err != nil {
				return err
			}
			defer cleanup()

			e, err := client.ApproveAction(context.Background(), &storev1.ApproveActionRequest{
				EntityId: args[0],
			})
			if err != nil {
				return fmt.Errorf("approve %s: %w", args[0], err)
			}

			fmt.Printf("Approved: %s (type=%s)\n", e.Id, e.Type)
			return nil
		},
	}
}

func denyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "deny <entity-id>",
		Short: "Deny a pending intercept action",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cleanup, err := dial()
			if err != nil {
				return err
			}
			defer cleanup()

			e, err := client.DenyAction(context.Background(), &storev1.DenyActionRequest{
				EntityId: args[0],
			})
			if err != nil {
				return fmt.Errorf("deny %s: %w", args[0], err)
			}

			fmt.Printf("Denied: %s (type=%s)\n", e.Id, e.Type)
			return nil
		},
	}
}

func componentNames(e *entityv1.Entity) string {
	if len(e.Components) == 0 {
		return "-"
	}
	names := ""
	for name := range e.Components {
		if names != "" {
			names += ","
		}
		names += name
	}
	return names
}
