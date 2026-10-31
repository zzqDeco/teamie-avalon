package game

import (
	"sync"

	"github.com/zzqDeco/teamie-avalon/internal/auth"
)

// Manager maintains isolated Avalon rooms keyed strictly by (workspaceId, conversationId).
// Read-only queries (GetView) and MCP protocol probes NEVER allocate rooms in memory.
type Manager struct {
	mu              sync.RWMutex
	timings         Timings
	rooms           map[RoomKey]*Room
	onNotifications func([]TargetedNotification)
	onRoomUpdated   func(workspaceID, conversationID string)
}

// NewManager creates an isolated Avalon room registry.
func NewManager(timings Timings) *Manager {
	return &Manager{
		timings: timings,
		rooms:   make(map[RoomKey]*Room),
	}
}

// SetCallbacks wires MCP SSE notification and Web SSE room-update hooks.
func (m *Manager) SetCallbacks(
	onNotifications func([]TargetedNotification),
	onRoomUpdated func(workspaceID, conversationID string),
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onNotifications = onNotifications
	m.onRoomUpdated = onRoomUpdated
}

// RoomCount returns the number of active rooms currently allocated in memory.
func (m *Manager) RoomCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.rooms)
}

// GetRoom looks up an existing room without creating one.
func (m *Manager) GetRoom(workspaceID, conversationID string) (*Room, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.rooms[RoomKey{WorkspaceID: workspaceID, ConversationID: conversationID}]
	return r, ok
}

// GetOrCreateRoom returns the room for (workspaceID, conversationID), creating it on first write action.
func (m *Manager) GetOrCreateRoom(workspaceID, conversationID string) *Room {
	key := RoomKey{WorkspaceID: workspaceID, ConversationID: conversationID}
	m.mu.RLock()
	if existing, ok := m.rooms[key]; ok {
		m.mu.RUnlock()
		return existing
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.rooms[key]; ok {
		return existing
	}
	room := NewRoom(
		workspaceID,
		conversationID,
		m.timings,
		func(notifs []TargetedNotification) {
			m.mu.RLock()
			fn := m.onNotifications
			m.mu.RUnlock()
			if fn != nil {
				fn(notifs)
			}
		},
		func(wsID, convID string) {
			m.mu.RLock()
			fn := m.onRoomUpdated
			m.mu.RUnlock()
			if fn != nil {
				fn(wsID, convID)
			}
		},
	)
	m.rooms[key] = room
	return room
}

// GetView returns the Fog-of-War-cropped view for claims without creating a room if none exists yet.
func (m *Manager) GetView(claims auth.Claims) PlayerView {
	if existing, ok := m.GetRoom(claims.WorkspaceID, claims.ConversationID); ok {
		return existing.GetView(claims)
	}
	// Ephemeral stack-only view: zero side effects for verifier probes or unstarted conversations.
	ephemeral := NewRoom(claims.WorkspaceID, claims.ConversationID, m.timings, nil, nil)
	return ephemeral.GetView(claims)
}

// CloseAll stops timers across all active rooms.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	rooms := make([]*Room, 0, len(m.rooms))
	for _, r := range m.rooms {
		rooms = append(rooms, r)
	}
	m.mu.Unlock()

	for _, r := range rooms {
		r.Close()
	}
}
