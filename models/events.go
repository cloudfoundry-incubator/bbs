package models

import (
	"code.cloudfoundry.org/bbs/format"
	"github.com/gogo/protobuf/proto"
)

type Event interface {
	EventType() string
	Key() string
	proto.Message
}

const (
	EventTypeInvalid = ""

	EventTypeDesiredLRPCreated = "desired_lrp_created"
	EventTypeDesiredLRPChanged = "desired_lrp_changed"
	EventTypeDesiredLRPRemoved = "desired_lrp_removed"

	EventTypeActualLRPCreated = "actual_lrp_created"
	EventTypeActualLRPChanged = "actual_lrp_changed"
	EventTypeActualLRPRemoved = "actual_lrp_removed"
	EventTypeActualLRPCrashed = "actual_lrp_crashed"

	EventTypeFlattenedActualLRPCreated = "flattened_actual_lrp_created"
	EventTypeFlattenedActualLRPChanged = "flattened_actual_lrp_changed"
	EventTypeFlattenedActualLRPRemoved = "flattened_actual_lrp_removed"

	EventTypeTaskCreated = "task_created"
	EventTypeTaskChanged = "task_changed"
	EventTypeTaskRemoved = "task_removed"
)

func VersionDesiredLRPsToV0(event Event) Event {
	switch event := event.(type) {
	case *DesiredLRPCreatedEvent:
		return NewDesiredLRPCreatedEvent(event.DesiredLrp.VersionDownTo(format.V0))
	case *DesiredLRPRemovedEvent:
		return NewDesiredLRPRemovedEvent(event.DesiredLrp.VersionDownTo(format.V0))
	case *DesiredLRPChangedEvent:
		return NewDesiredLRPChangedEvent(
			event.Before.VersionDownTo(format.V0),
			event.After.VersionDownTo(format.V0),
		)
	default:
		return event
	}
}

func NewDesiredLRPCreatedEvent(desiredLRP *DesiredLRP) *DesiredLRPCreatedEvent {
	return &DesiredLRPCreatedEvent{
		DesiredLrp: desiredLRP,
	}
}

func (event *DesiredLRPCreatedEvent) EventType() string {
	return EventTypeDesiredLRPCreated
}

func (event *DesiredLRPCreatedEvent) Key() string {
	return event.DesiredLrp.GetProcessGuid()
}

func NewDesiredLRPChangedEvent(before, after *DesiredLRP) *DesiredLRPChangedEvent {
	return &DesiredLRPChangedEvent{
		Before: before,
		After:  after,
	}
}

func (event *DesiredLRPChangedEvent) EventType() string {
	return EventTypeDesiredLRPChanged
}

func (event *DesiredLRPChangedEvent) Key() string {
	return event.Before.GetProcessGuid()
}

func NewDesiredLRPRemovedEvent(desiredLRP *DesiredLRP) *DesiredLRPRemovedEvent {
	return &DesiredLRPRemovedEvent{
		DesiredLrp: desiredLRP,
	}
}

func (event *DesiredLRPRemovedEvent) EventType() string {
	return EventTypeDesiredLRPRemoved
}

func (event DesiredLRPRemovedEvent) Key() string {
	return event.DesiredLrp.GetProcessGuid()
}

func NewActualLRPChangedEvent(before, after *ActualLRPGroup) *ActualLRPChangedEvent {
	return &ActualLRPChangedEvent{
		Before: before,
		After:  after,
	}
}

func (event *ActualLRPChangedEvent) EventType() string {
	return EventTypeActualLRPChanged
}

func (event *ActualLRPChangedEvent) Key() string {
	actualLRP, _ := event.Before.Resolve()
	return actualLRP.GetInstanceGuid()
}

func NewFlattenedActualLRPChangedEvent(before, after *ActualLRP) *FlattenedActualLRPChangedEvent {
	// if before.ActualLRPKey != after.ActualLRPKey || before.ActualLRPInstanceKey != after.ActualLRPInstanceKey {
	// 	return nil
	// }
	if before == nil && after == nil {
		return nil
	}
	event := &FlattenedActualLRPChangedEvent{}
	if before != nil {
		event.Before = &ActualLRPInfo{
			ActualLRPNetInfo: before.ActualLRPNetInfo,
			CrashCount:       before.CrashCount,
			CrashReason:      before.CrashReason,
			State:            before.State,
			PlacementState:   before.PlacementState,
			PlacementError:   before.PlacementError,
			Since:            before.Since,
			ModificationTag:  before.ModificationTag,
		}
	}
	if after != nil {
		event.ActualLrpInstanceKey = &after.ActualLRPInstanceKey
		event.ActualLrpKey = &after.ActualLRPKey
		event.After = &ActualLRPInfo{
			ActualLRPNetInfo: after.ActualLRPNetInfo,
			CrashCount:       after.CrashCount,
			CrashReason:      after.CrashReason,
			State:            after.State,
			PlacementState:   after.PlacementState,
			PlacementError:   after.PlacementError,
			Since:            after.Since,
			ModificationTag:  after.ModificationTag,
		}
	} else {
		event.ActualLrpInstanceKey = &before.ActualLRPInstanceKey
		event.ActualLrpKey = &before.ActualLRPKey
	}

	return event
}

func (event *FlattenedActualLRPChangedEvent) EventType() string {
	return EventTypeFlattenedActualLRPChanged
}

func (event *FlattenedActualLRPChangedEvent) Key() string {
	return event.ActualLrpInstanceKey.GetInstanceGuid()
}

func (event *FlattenedActualLRPChangedEvent) BeforeAndAfter() (*ActualLRP, *ActualLRP) {
	if event == nil {
		return nil, nil
	}
	before := new(ActualLRP)
	after := new(ActualLRP)
	if event.ActualLrpKey != nil {
		before.ActualLRPKey = *event.ActualLrpKey
		after.ActualLRPKey = *event.ActualLrpKey
	}
	if event.ActualLrpInstanceKey != nil {
		before.ActualLRPInstanceKey = *event.ActualLrpInstanceKey
		after.ActualLRPInstanceKey = *event.ActualLrpInstanceKey
	}
	if event.Before != nil {
		before.ActualLRPNetInfo = event.Before.ActualLRPNetInfo
		before.CrashCount = event.Before.CrashCount
		before.CrashReason = event.Before.CrashReason
		before.State = event.Before.State
		before.PlacementState = event.Before.PlacementState
		before.PlacementError = event.Before.PlacementError
		before.Since = event.Before.Since
		before.ModificationTag = event.Before.ModificationTag
	}
	if event.After != nil {
		after.ActualLRPNetInfo = event.After.ActualLRPNetInfo
		after.CrashCount = event.After.CrashCount
		after.CrashReason = event.After.CrashReason
		after.State = event.After.State
		after.PlacementState = event.After.PlacementState
		after.PlacementError = event.After.PlacementError
		after.Since = event.After.Since
		after.ModificationTag = event.After.ModificationTag
	}
	return before, after
}

func NewActualLRPCrashedEvent(before, after *ActualLRP) *ActualLRPCrashedEvent {
	return &ActualLRPCrashedEvent{
		ActualLRPKey:         after.ActualLRPKey,
		ActualLRPInstanceKey: before.ActualLRPInstanceKey,
		CrashCount:           after.CrashCount,
		CrashReason:          after.CrashReason,
		Since:                after.Since,
	}
}

func (event *ActualLRPCrashedEvent) EventType() string {
	return EventTypeActualLRPCrashed
}

func (event *ActualLRPCrashedEvent) Key() string {
	return event.ActualLRPInstanceKey.InstanceGuid
}

func NewFlattenedActualLRPCrashedEvent(before, after *ActualLRP) *ActualLRPCrashedEvent {
	return &ActualLRPCrashedEvent{
		ActualLRPKey:         after.ActualLRPKey,
		ActualLRPInstanceKey: before.ActualLRPInstanceKey,
		CrashCount:           after.CrashCount,
		CrashReason:          after.CrashReason,
		Since:                after.Since,
	}
}

func NewActualLRPRemovedEvent(actualLRPGroup *ActualLRPGroup) *ActualLRPRemovedEvent {
	return &ActualLRPRemovedEvent{
		ActualLrpGroup: actualLRPGroup,
	}
}

func (event *ActualLRPRemovedEvent) EventType() string {
	return EventTypeActualLRPRemoved
}

func (event *ActualLRPRemovedEvent) Key() string {
	actualLRP, _ := event.ActualLrpGroup.Resolve()
	return actualLRP.GetInstanceGuid()
}

func NewFlattenedActualLRPRemovedEvent(actualLRP *ActualLRP) *FlattenedActualLRPRemovedEvent {
	return &FlattenedActualLRPRemovedEvent{
		ActualLrp: actualLRP,
	}
}

func (event *FlattenedActualLRPRemovedEvent) EventType() string {
	return EventTypeFlattenedActualLRPRemoved
}

func (event *FlattenedActualLRPRemovedEvent) Key() string {
	return event.ActualLrp.GetInstanceGuid()
}

func NewActualLRPCreatedEvent(actualLRPGroup *ActualLRPGroup) *ActualLRPCreatedEvent {
	return &ActualLRPCreatedEvent{
		ActualLrpGroup: actualLRPGroup,
	}
}

func (event *ActualLRPCreatedEvent) EventType() string {
	return EventTypeActualLRPCreated
}

func (event *ActualLRPCreatedEvent) Key() string {
	actualLRP, _ := event.ActualLrpGroup.Resolve()
	return actualLRP.GetInstanceGuid()
}

func NewFlattenedActualLRPCreatedEvent(actualLRP *ActualLRP) *FlattenedActualLRPCreatedEvent {
	return &FlattenedActualLRPCreatedEvent{
		ActualLrp: actualLRP,
	}
}

func (event *FlattenedActualLRPCreatedEvent) EventType() string {
	return EventTypeFlattenedActualLRPCreated
}

func (event *FlattenedActualLRPCreatedEvent) Key() string {
	return event.ActualLrp.GetInstanceGuid()
}

func (request *EventsByCellId) Validate() error {
	return nil
}

func NewTaskCreatedEvent(task *Task) *TaskCreatedEvent {
	return &TaskCreatedEvent{
		Task: task,
	}
}

func (event *TaskCreatedEvent) EventType() string {
	return EventTypeTaskCreated
}

func (event *TaskCreatedEvent) Key() string {
	return event.Task.GetTaskGuid()
}

func NewTaskChangedEvent(before, after *Task) *TaskChangedEvent {
	return &TaskChangedEvent{
		Before: before,
		After:  after,
	}
}

func (event *TaskChangedEvent) EventType() string {
	return EventTypeTaskChanged
}

func (event *TaskChangedEvent) Key() string {
	return event.Before.GetTaskGuid()
}

func NewTaskRemovedEvent(task *Task) *TaskRemovedEvent {
	return &TaskRemovedEvent{
		Task: task,
	}
}

func (event *TaskRemovedEvent) EventType() string {
	return EventTypeTaskRemoved
}

func (event TaskRemovedEvent) Key() string {
	return event.Task.GetTaskGuid()
}
