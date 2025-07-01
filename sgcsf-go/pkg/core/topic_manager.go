package core

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sgcsf/sgcsf-go/internal/types"
)

// topicManager implements TopicManager interface
type topicManager struct {
	topics        map[string]*Topic
	subscriptions map[string]*types.Subscription
	mutex         sync.RWMutex
}

// NewTopicManager creates a new topic manager
func NewTopicManager() TopicManager {
	return &topicManager{
		topics:        make(map[string]*Topic),
		subscriptions: make(map[string]*types.Subscription),
	}
}

// CreateTopic creates a new topic
func (tm *topicManager) CreateTopic(name string, config *types.TopicConfig) error {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	if _, exists := tm.topics[name]; exists {
		return fmt.Errorf("topic already exists: %s", name)
	}

	now := time.Now().Unix()
	topic := &Topic{
		Config:       config,
		Subscribers:  make([]*types.Subscription, 0),
		MessageCount: 0,
		TotalSize:    0,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	tm.topics[name] = topic
	return nil
}

// DeleteTopic deletes a topic
func (tm *topicManager) DeleteTopic(name string) error {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	topic, exists := tm.topics[name]
	if !exists {
		return fmt.Errorf("topic not found: %s", name)
	}

	// Remove all subscriptions for this topic
	for _, subscription := range topic.Subscribers {
		delete(tm.subscriptions, subscription.ID)
	}

	delete(tm.topics, name)
	return nil
}

// GetTopic retrieves a topic
func (tm *topicManager) GetTopic(name string) (*Topic, error) {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()

	topic, exists := tm.topics[name]
	if !exists {
		return nil, fmt.Errorf("topic not found: %s", name)
	}

	return topic, nil
}

// ListTopics lists all topics
func (tm *topicManager) ListTopics() []*Topic {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()

	topics := make([]*Topic, 0, len(tm.topics))
	for _, topic := range tm.topics {
		topics = append(topics, topic)
	}

	return topics
}

// TopicExists checks if a topic exists
func (tm *topicManager) TopicExists(name string) bool {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()

	_, exists := tm.topics[name]
	return exists
}

// Subscribe creates a subscription to a topic
func (tm *topicManager) Subscribe(clientID string, topicPattern string, handler types.MessageHandler) (*types.Subscription, error) {
	subscription := &types.Subscription{
		ID:          types.GenerateSubscriptionID(),
		Topic:       topicPattern,
		ClientID:    clientID,
		QoS:         types.QoSAtLeastOnce,
		CreatedAt:   time.Now().Unix(),
		LastMessage: 0,
		Metadata:    make(map[string]string),
		Handler:     handler,
	}

	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	tm.subscriptions[subscription.ID] = subscription

	// Add subscription to matching topics
	for topicName, topic := range tm.topics {
		if tm.MatchTopic(topicPattern, topicName) {
			topic.Subscribers = append(topic.Subscribers, subscription)
			topic.UpdatedAt = time.Now().Unix()
		}
	}

	return subscription, nil
}

// Unsubscribe removes a subscription
func (tm *topicManager) Unsubscribe(subscriptionID string) error {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	_, exists := tm.subscriptions[subscriptionID]
	if !exists {
		return fmt.Errorf("subscription not found: %s", subscriptionID)
	}

	// Remove from topics
	for _, topic := range tm.topics {
		for i, sub := range topic.Subscribers {
			if sub.ID == subscriptionID {
				topic.Subscribers = append(topic.Subscribers[:i], topic.Subscribers[i+1:]...)
				topic.UpdatedAt = time.Now().Unix()
				break
			}
		}
	}

	delete(tm.subscriptions, subscriptionID)
	return nil
}

// GetSubscription retrieves a subscription
func (tm *topicManager) GetSubscription(subscriptionID string) (*types.Subscription, error) {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()

	subscription, exists := tm.subscriptions[subscriptionID]
	if !exists {
		return nil, fmt.Errorf("subscription not found: %s", subscriptionID)
	}

	return subscription, nil
}

// ListSubscriptions lists all subscriptions for a client
func (tm *topicManager) ListSubscriptions(clientID string) []*types.Subscription {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()

	subscriptions := make([]*types.Subscription, 0)
	for _, subscription := range tm.subscriptions {
		if subscription.ClientID == clientID {
			subscriptions = append(subscriptions, subscription)
		}
	}

	return subscriptions
}

// GetTopicSubscribers gets all subscribers for a topic
func (tm *topicManager) GetTopicSubscribers(topicName string) []*types.Subscription {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()

	var subscribers []*types.Subscription

	// Direct topic match
	if topic, exists := tm.topics[topicName]; exists {
		subscribers = append(subscribers, topic.Subscribers...)
	}

	// Pattern-based matching for subscriptions
	for _, subscription := range tm.subscriptions {
		if tm.MatchTopic(subscription.Topic, topicName) {
			// Check if already added from direct topic match
			found := false
			for _, existing := range subscribers {
				if existing.ID == subscription.ID {
					found = true
					break
				}
			}
			if !found {
				subscribers = append(subscribers, subscription)
			}
		}
	}

	return subscribers
}

// RouteMessage routes a message to appropriate subscriptions
func (tm *topicManager) RouteMessage(message *types.SGCSFMessage) ([]*types.Subscription, error) {
	subscribers := tm.GetTopicSubscribers(message.Topic)

	// Update topic statistics
	tm.mutex.Lock()
	if topic, exists := tm.topics[message.Topic]; exists {
		topic.MessageCount++
		topic.TotalSize += int64(len(message.Payload))
		topic.UpdatedAt = time.Now().Unix()
	}
	tm.mutex.Unlock()

	return subscribers, nil
}

// MatchTopic checks if a topic pattern matches a topic name
func (tm *topicManager) MatchTopic(pattern string, topic string) bool {
	// Handle wildcard patterns
	// + matches a single level
	// # matches multiple levels
	// * matches any characters within a level

	// Simple implementation for common patterns
	if pattern == topic {
		return true
	}

	// Handle + wildcard (single level)
	if strings.Contains(pattern, "+") {
		patternParts := strings.Split(pattern, "/")
		topicParts := strings.Split(topic, "/")

		if len(patternParts) != len(topicParts) {
			return false
		}

		for i, part := range patternParts {
			if part != "+" && part != topicParts[i] {
				return false
			}
		}
		return true
	}

	// Handle # wildcard (multi-level)
	if strings.HasSuffix(pattern, "#") {
		prefix := strings.TrimSuffix(pattern, "#")
		prefix = strings.TrimSuffix(prefix, "/")
		return strings.HasPrefix(topic, prefix)
	}

	// Handle * wildcard within level
	if strings.Contains(pattern, "*") {
		matched, _ := filepath.Match(pattern, topic)
		return matched
	}

	return false
}

// ensureTopicExists creates a topic if it doesn't exist
func (tm *topicManager) ensureTopicExists(name string) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	if _, exists := tm.topics[name]; !exists {
		now := time.Now().Unix()
		topic := &Topic{
			Config: &types.TopicConfig{
				Name:       name,
				Type:       types.TopicUnicast,
				Persistent: false,
				Retention:  3600, // 1 hour default
				MaxSize:    10 * 1024 * 1024, // 10MB default
				QoS:        types.QoSAtLeastOnce,
				Metadata:   make(map[string]string),
			},
			Subscribers:  make([]*types.Subscription, 0),
			MessageCount: 0,
			TotalSize:    0,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		tm.topics[name] = topic
	}
}