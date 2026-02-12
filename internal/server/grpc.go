package server

import (
	"context"

	entityv1 "github.com/boshu2/lattice-lab/gen/entity/v1"
	storev1 "github.com/boshu2/lattice-lab/gen/store/v1"
	"github.com/boshu2/lattice-lab/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Server implements the EntityStoreService gRPC interface.
type Server struct {
	storev1.UnimplementedEntityStoreServiceServer
	store *store.Store
}

// New creates a gRPC server backed by the given store.
func New(s *store.Store) *Server {
	return &Server{store: s}
}

func (s *Server) CreateEntity(_ context.Context, req *storev1.CreateEntityRequest) (*entityv1.Entity, error) {
	if req.Entity == nil {
		return nil, status.Error(codes.InvalidArgument, "entity is required")
	}
	if req.Entity.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "entity id is required")
	}

	e, err := s.store.Create(req.Entity)
	if err != nil {
		return nil, status.Errorf(codes.AlreadyExists, "%v", err)
	}
	return e, nil
}

func (s *Server) GetEntity(_ context.Context, req *storev1.GetEntityRequest) (*entityv1.Entity, error) {
	e, err := s.store.Get(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return e, nil
}

func (s *Server) ListEntities(_ context.Context, req *storev1.ListEntitiesRequest) (*storev1.ListEntitiesResponse, error) {
	entities := s.store.List(req.TypeFilter)
	return &storev1.ListEntitiesResponse{Entities: entities}, nil
}

func (s *Server) UpdateEntity(_ context.Context, req *storev1.UpdateEntityRequest) (*entityv1.Entity, error) {
	if req.Entity == nil {
		return nil, status.Error(codes.InvalidArgument, "entity is required")
	}

	e, err := s.store.Update(req.Entity)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return e, nil
}

func (s *Server) DeleteEntity(_ context.Context, req *storev1.DeleteEntityRequest) (*emptypb.Empty, error) {
	if err := s.store.Delete(req.Id); err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) ApproveAction(_ context.Context, req *storev1.ApproveActionRequest) (*entityv1.Entity, error) {
	return nil, status.Error(codes.Unimplemented, "approval gate not wired to this server instance")
}

func (s *Server) DenyAction(_ context.Context, req *storev1.DenyActionRequest) (*entityv1.Entity, error) {
	return nil, status.Error(codes.Unimplemented, "approval gate not wired to this server instance")
}

func (s *Server) WatchEntities(req *storev1.WatchEntitiesRequest, stream grpc.ServerStreamingServer[storev1.EntityEvent]) error {
	w := s.store.Watch(req.TypeFilter)
	defer s.store.Unwatch(w)

	for {
		select {
		case event, ok := <-w.Events:
			if !ok {
				return nil
			}
			if err := stream.Send(event); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
}
