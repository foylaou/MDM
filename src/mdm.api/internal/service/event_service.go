package service

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	mdmv1 "github.com/anthropics/mdm-server/gen/mdm/v1"
	"github.com/anthropics/mdm-server/gen/mdm/v1/mdmv1connect"
	"github.com/anthropics/mdm-server/internal/port"
)

type EventService struct {
	mdmv1connect.UnimplementedEventServiceHandler
	broker port.EventBroker
}

func NewEventService(broker port.EventBroker) *EventService {
	return &EventService{broker: broker}
}

func (s *EventService) StreamEvents(ctx context.Context, req *connect.Request[mdmv1.StreamEventsRequest], stream *connect.ServerStream[mdmv1.MDMEvent]) error {
	ch := s.broker.Subscribe(ctx)
	for evt := range ch {
		if req.Msg.FilterUdid != "" && evt.UDID != req.Msg.FilterUdid {
			continue
		}
		if err := stream.Send(&mdmv1.MDMEvent{
			Id:          evt.ID,
			EventType:   evt.EventType,
			Udid:        evt.UDID,
			CommandUuid: evt.CommandUUID,
			Status:      evt.Status,
			RawPayload:  evt.RawPayload,
			Timestamp:   timestamppb.New(evt.Timestamp),
		}); err != nil {
			return err
		}
	}
	return nil
}
