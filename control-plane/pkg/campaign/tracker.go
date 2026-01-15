// Package campaign provides campaign management for the control plane.
package campaign

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/yourorg/control-plane/pkg/db/models"
)

// Tracker tracks campaign progress
type Tracker struct {
	mu              sync.RWMutex
	db              *gorm.DB
	logger          *zap.Logger
	activeCampaigns map[string]*CampaignState
}

// CampaignState represents the state of an active campaign
type CampaignState struct {
	CampaignID     string
	CurrentPhase   int
	TotalPhases    int
	TotalAgents    int
	ProcessedAgents int
	SuccessCount   int
	FailureCount   int
	StartedAt      time.Time
	LastUpdate     time.Time
}

// NewTracker creates a new campaign tracker
func NewTracker(db *gorm.DB, logger *zap.Logger) *Tracker {
	return &Tracker{
		db:              db,
		logger:          logger,
		activeCampaigns: make(map[string]*CampaignState),
	}
}

// StartTracking starts tracking a campaign
func (t *Tracker) StartTracking(ctx context.Context, campaignID string) error {
	var campaign models.Campaign
	if err := t.db.Preload("Phases").First(&campaign, "id = ?", campaignID).Error; err != nil {
		return err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.activeCampaigns[campaignID] = &CampaignState{
		CampaignID:  campaignID,
		TotalPhases: len(campaign.Phases),
		StartedAt:   time.Now(),
		LastUpdate:  time.Now(),
	}

	t.logger.Info("started tracking campaign",
		zap.String("campaign_id", campaignID))

	return nil
}

// StopTracking stops tracking a campaign
func (t *Tracker) StopTracking(campaignID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.activeCampaigns, campaignID)
}

// UpdateProgress updates campaign progress
func (t *Tracker) UpdateProgress(campaignID string, successCount, failureCount int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	state, ok := t.activeCampaigns[campaignID]
	if !ok {
		return
	}

	state.SuccessCount = successCount
	state.FailureCount = failureCount
	state.ProcessedAgents = successCount + failureCount
	state.LastUpdate = time.Now()
}

// GetState returns the current state of a campaign
func (t *Tracker) GetState(campaignID string) *CampaignState {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if state, ok := t.activeCampaigns[campaignID]; ok {
		// Return a copy
		stateCopy := *state
		return &stateCopy
	}
	return nil
}

// GetAllActive returns all active campaign states
func (t *Tracker) GetAllActive() []*CampaignState {
	t.mu.RLock()
	defer t.mu.RUnlock()

	states := make([]*CampaignState, 0, len(t.activeCampaigns))
	for _, state := range t.activeCampaigns {
		stateCopy := *state
		states = append(states, &stateCopy)
	}
	return states
}

// RecordExecution records an execution result
func (t *Tracker) RecordExecution(ctx context.Context, campaignID, agentID string, success bool) {
	t.mu.Lock()
	state, ok := t.activeCampaigns[campaignID]
	t.mu.Unlock()

	if !ok {
		return
	}

	t.mu.Lock()
	if success {
		state.SuccessCount++
	} else {
		state.FailureCount++
	}
	state.ProcessedAgents++
	state.LastUpdate = time.Now()
	t.mu.Unlock()

	// Update phase progress in database
	var currentPhase models.CampaignPhase
	if err := t.db.Where("campaign_id = ? AND status = ?", campaignID, models.PhaseStatusRunning).First(&currentPhase).Error; err == nil {
		updates := map[string]interface{}{}
		if success {
			updates["success_count"] = gorm.Expr("success_count + 1")
		} else {
			updates["failure_count"] = gorm.Expr("failure_count + 1")
		}
		t.db.Model(&currentPhase).Updates(updates)
	}
}

// GetProgress calculates current progress percentage
func (t *Tracker) GetProgress(campaignID string) float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	state, ok := t.activeCampaigns[campaignID]
	if !ok || state.TotalAgents == 0 {
		return 0
	}

	return float64(state.ProcessedAgents) / float64(state.TotalAgents) * 100
}

// GetSuccessRate calculates current success rate
func (t *Tracker) GetSuccessRate(campaignID string) float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	state, ok := t.activeCampaigns[campaignID]
	if !ok || state.ProcessedAgents == 0 {
		return 0
	}

	return float64(state.SuccessCount) / float64(state.ProcessedAgents) * 100
}

// ShouldTriggerRollback checks if rollback should be triggered
func (t *Tracker) ShouldTriggerRollback(campaignID string, threshold float64) bool {
	successRate := t.GetSuccessRate(campaignID)
	state := t.GetState(campaignID)

	// Only check after a minimum number of executions
	if state == nil || state.ProcessedAgents < 5 {
		return false
	}

	return successRate < threshold
}
