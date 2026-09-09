package notification

import (
	notification "FeedFlow/internal/notification/model"
	"context"
	"errors"
	"fmt"
)

var ErrSenderNotRegistered = errors.New(
	"notification sender is not registered",
)

type Sender interface {
	Send(ctx context.Context, msg notification.Message) error
}

type Registry map[notification.ChannelType]Sender

func NewRegistry(sender map[notification.ChannelType]Sender) (Registry, error) {
	registry := make(Registry, len(sender))
	for channelType, channelSender := range sender {
		if channelType == "" {
			return nil, fmt.Errorf("notification sender must have a channel type")
		}
		if channelSender == nil {
			return nil, fmt.Errorf("sender for channel %q is nil", channelType)
		}
		registry[channelType] = channelSender
	}
	return registry, nil
}

func (r Registry) Get(channelType notification.ChannelType) (Sender, error) {
	channelSender, ok := r[channelType]
	if !ok || channelSender == nil {
		return nil, fmt.Errorf("%w: %s", ErrSenderNotRegistered, channelType)
	}
	return channelSender, nil
}
