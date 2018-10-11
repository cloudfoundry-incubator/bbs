package controllers

import (
	"sort"

	"code.cloudfoundry.org/bbs/events"
	"code.cloudfoundry.org/bbs/models"
)

// LRP Instance Constraints:
// - an ActualLRP is identified by its ActualLRPKey + ActualLRPInstanceKey
// - as long as these don't change, any change in presence or state will result in a changed event
// - an ActualLRP transitioning from Unclaimed -> Claimed will result in a
//   changed event (this is not exactly an exception since an ActualLRPInstanceKey
//   doesn't have a defined identity by this constraint)
// - an ActualLRP cannot transition directly from having an
//   ActualLRPInstanceKey to having none (i.e. Running -> Unclaimed). This must
//   result in a removed and a created event.
//
// LRP Group emissions:
// - If the slot changes from nil to something, that's an ActualLRPCreated event
// - If the slot changes from something to nil, that's an ActualLRPRemoved event
// - If the Instance slot changes from one ActualLRPInstanceKey to another,
//   it's considered an ActualLRPCreated and ActualLRPRemoved event
// - If the instance slot does not change the ActualLRPInstanceKey, it's
//   considered an ActualLRPChanged event (including the transition from Running ->
//   Unclaimed, e.g. when the LRP crashes)
// - If the instance slot changes from Unclaimed to another state, it's still considered a changed event
//
// General:
//  - We should emit Crashed events followed by Create or Changed events where
//    the resulting LRP is in the running state before Remove events
//
// Events that follow are instance events. LRP group events go through a separate algorithm.
//
// ClaimActualLRP
//  Changes Allowed:
//      - Unclaimed -> Claimed (Changed event)
//      - Running -> Claimed (Changed event)
//      - Crashed -> Claimed (Changed event)
//      - Claimed -> Claimed (No event)
//
//  Transition is only allowed if LRPInstanceKey will be the same or if the LRP
//  is Unclaimed. No case this will result in any other event.
//
// UnclaimActualLRP
//  Changes Allowed:
//      Not Unclaimed -> Unclaimed (Should emit removed and created event)
//
// StartActualLRP
//  Changes Allowed:
//      nil -> Running (Created event)
//      Unclaimed -> Running (if instanceKey matches) (changed event)
//      Claimed -> Running (if instanceKey matches) (changed event)
//      Running -> Running (if instanceKey matches) (changed event) (Only allowed if netInfo has changed)
//
//      if the lrp being started is suspect:
//          do nothing
//      if suspect exists and it is not the lrp being started:
//          emit removed event for the suspect LRP
//
// CrashActualLRP
//  Changes Allowed:
//      Claimed -> Crashed (if instanceKey matches) (changed event)
//      Running -> Crashed (if instanceKey matches) (changed event)
//
//      Claimed -> Unclaimed  (if instanceKey matches) (if crashedCount is below) (created + removed event)
//      Running -> Unclaimed  (if instanceKey matches) (if crashCount is below) (created + removed event)
//
// FailActualLRP
//  Unclaimed -> Unclaimed (no events emitted)
//
// RemoveActualLRP
//  removed event

type EventCalculator struct {
	ActualLRPGroupHub    events.Hub
	ActualLRPInstanceHub events.Hub
}

// EmitEvents emits the events such as when the changes identified in the
// events are applied to the beforeSet the resulting state is equal to
// afterSet.  The beforeSet and afterSet are assumed to have the same process
// guid and index.
func (e EventCalculator) EmitEvents(beforeSet, afterSet []*models.ActualLRP) {
	events := []models.Event{}

	beforeGroup := models.ResolveActualLRPGroup(beforeSet)
	afterGroup := models.ResolveActualLRPGroup(removeNilLRPs(afterSet))

	for _, ev := range generateLRPGroupEvents(beforeGroup, afterGroup) {
		e.ActualLRPGroupHub.Emit(ev)
	}

	// stretch the two slices to be of equal size.  make sure we do this after
	// emitting the group events, otherwise ResolveActualLRPGroup will panic if
	// it encounters nil lrps.
	stretchSlice(&beforeSet, &afterSet)

	for i := range afterSet {
		events = append(events, generateLRPInstanceEvents(beforeSet[i], afterSet[i])...)
	}

	sort.Slice(events, func(i, j int) bool {
		return shouldEventComeFirst(events[i], events[j])
	})

	for _, ev := range events {
		e.ActualLRPInstanceHub.Emit(ev)
	}
}

// RecordChange returns a new LRP set with the before LRP replaced with after
// LRP.  The index of after and before is the same.  New LRPs (i.e. when before
// is nil) are appended to the end of the lrp slice.
func (e EventCalculator) RecordChange(before, after *models.ActualLRP, lrps []*models.ActualLRP) []*models.ActualLRP {
	found := false
	newLRPs := []*models.ActualLRP{}
	for _, l := range lrps {
		if l == nil {
			// this entry is recording a LRP removal, just skip it
			newLRPs = append(newLRPs, nil)
			continue
		}

		if before != nil && l.ActualLRPInstanceKey.Equal(before.ActualLRPInstanceKey) {
			newLRPs = append(newLRPs, after)
			found = true
		} else {
			newLRPs = append(newLRPs, l)
		}
	}

	if !found {
		return append(lrps, after)
	}

	return newLRPs
}

func generateCrashedInstanceEvents(before, after *models.ActualLRP) []models.Event {
	return wrapEvent(
		models.NewActualLRPCrashedEvent(before, after),
		models.NewActualLRPInstanceChangedEvent(before, after),
	)
}

func generateUpdateInstanceEvents(before, after *models.ActualLRP) []models.Event {
	return wrapEvent(
		models.NewActualLRPInstanceChangedEvent(before, after),
	)
}

func generateUnclaimedInstanceEvents(before, after *models.ActualLRP) []models.Event {
	events := []models.Event{}

	// This LRP probably transitioned from Claimed/Running -> Crashed ->
	// Unclaimed because it was restartable
	if after.CrashCount > before.CrashCount {
		events = append(events, models.NewActualLRPCrashedEvent(before, after))
	}

	return append(
		events,
		models.NewActualLRPInstanceCreatedEvent(after),
		models.NewActualLRPInstanceRemovedEvent(before),
	)
}

func generateLRPInstanceEvents(before, after *models.ActualLRP) []models.Event {
	if before.Equal(after) {
		// nothing changed
		return nil
	}

	if after == nil {
		return wrapEvent(models.NewActualLRPInstanceRemovedEvent(before))
	}

	if before == nil {
		return wrapEvent(models.NewActualLRPInstanceCreatedEvent(after))
	}

	switch after.State {
	case models.ActualLRPStateUnclaimed:
		return generateUnclaimedInstanceEvents(before, after)
	case models.ActualLRPStateCrashed:
		return generateCrashedInstanceEvents(before, after)
	case models.ActualLRPStateRunning:
		return generateUpdateInstanceEvents(before, after)
	default:
		return nil
	}
}

func generateCrashedGroupEvents(before, after *models.ActualLRP) []models.Event {
	return wrapEvent(
		models.NewActualLRPCrashedEvent(before, after),
		models.NewActualLRPChangedEvent(before.ToActualLRPGroup(), after.ToActualLRPGroup()),
	)
}

func generateUnclaimedGroupEvents(before, after *models.ActualLRP) []models.Event {
	events := []models.Event{}

	// This LRP probably transitioned from Claimed/Running -> Crashed ->
	// Unclaimed because it was restartable
	if after.CrashCount > before.CrashCount {
		events = append(events, models.NewActualLRPCrashedEvent(before, after))
	}

	return append(
		events,
		models.NewActualLRPChangedEvent(before.ToActualLRPGroup(), after.ToActualLRPGroup()),
	)
}

func generateUpdateGroupEvents(before, after *models.ActualLRP) []models.Event {
	if !after.ActualLRPInstanceKey.Equal(before.ActualLRPInstanceKey) {
		// an Ordinary LRP replaced Suspect LRP
		return wrapEvent(
			models.NewActualLRPCreatedEvent(after.ToActualLRPGroup()),
			models.NewActualLRPRemovedEvent(before.ToActualLRPGroup()),
		)
	}

	return nil
}

// The main difference between this function and generateLRPInstanceEvents
// (besides using different event types) is that the latter generates a
// remove+create events when the LRP is unclaimed.  This function return a
// ActualLRPChangedEvent instead to be compatible with olf subscribers.
func generateLRPInstanceGroupEvents(before, after *models.ActualLRP) []models.Event {
	if before.Equal(after) {
		// nothing changed
		return nil
	}

	if after == nil {
		return wrapEvent(models.NewActualLRPRemovedEvent(before.ToActualLRPGroup()))
	}

	if before == nil {
		return wrapEvent(models.NewActualLRPCreatedEvent(after.ToActualLRPGroup()))
	}

	switch after.State {
	case models.ActualLRPStateUnclaimed:
		return generateUnclaimedGroupEvents(before, after)
	case models.ActualLRPStateClaimed:
		return generateUpdateGroupEvents(before, after)
	case models.ActualLRPStateRunning:
		return generateUpdateGroupEvents(before, after)
	case models.ActualLRPStateCrashed:
		return generateCrashedGroupEvents(before, after)
	default:
		return nil
	}
}

// return the resulting lrp of the given event, that is the lrp being created
// or the lrp in the new lrp in a ActualLRPChanged event.  Returns nil for
// crashed and removed events.  Returns true iff this is a crashed event.
func getEventLRP(e models.Event) (*models.ActualLRP, bool) {
	switch x := e.(type) {
	case *models.ActualLRPCreatedEvent:
		lrp, _, _ := x.ActualLrpGroup.Resolve()
		return lrp, false
	case *models.ActualLRPChangedEvent:
		lrp, _, _ := x.After.Resolve()
		return lrp, false
	case *models.ActualLRPInstanceCreatedEvent:
		return x.ActualLrp, false
	case *models.ActualLRPInstanceChangedEvent:
		return x.After, false
	case *models.ActualLRPCrashedEvent:
		return nil, true
	}

	return nil, false
}

// determine if i should be emitted before j.  The ordering ensures continuous
// routability, so events with running instances should be emitted first
// followed by remove/crash events.
func shouldEventComeFirst(i, j models.Event) bool {
	first, firstCrashed := getEventLRP(i)
	second, secondCrashed := getEventLRP(j)

	// sort crashed events first to be backward compatible with the old
	// event stream which emitted the crashed event before the
	// remove/changed events.
	if firstCrashed {
		return !secondCrashed
	}

	if secondCrashed {
		return false
	}

	// i is a removed event.  These should be emitted last, so i cannot less
	// than j.
	if first == nil {
		return false
	}

	// j is a removed event.  These should be emitted last, so i is less than
	// (emitted before) j.
	if second == nil {
		return true
	}

	if first.State == models.ActualLRPStateRunning {
		// true if the lrp from the other event isn't running, otherwise
		// treat them as equal
		return second.State != models.ActualLRPStateRunning
	}

	return false
}

func generateLRPGroupEvents(before, after *models.ActualLRPGroup) []models.Event {
	events := generateLRPInstanceGroupEvents(before.Instance, after.Instance)
	events = append(events, generateLRPInstanceGroupEvents(before.Evacuating, after.Evacuating)...)

	sort.Slice(events, func(i, j int) bool {
		return shouldEventComeFirst(events[i], events[j])
	})

	return events
}

// A Helper function to remove null lrps that could be added to the set if an
// LRP is removed.
func removeNilLRPs(lrps []*models.ActualLRP) []*models.ActualLRP {
	newLRPs := []*models.ActualLRP{}
	for _, l := range lrps {
		if l == nil {
			continue
		}
		newLRPs = append(newLRPs, l)
	}
	return newLRPs
}

func stretchSlice(before, after *[]*models.ActualLRP) {
	if len(*before) < len(*after) {
		newLRPs := make([]*models.ActualLRP, len(*after))
		copy(newLRPs, *before)
		*before = newLRPs
	}
}

func wrapEvent(e ...models.Event) []models.Event {
	return e
}
