package queen

import (
	"time"

	"example.com/apiary-colony-service/internal/domain"
	"example.com/apiary-colony-service/internal/store"
)

const (
	ReasonAging      = "aging"
	ReasonAggression = "aggression"
	ReasonLoss       = "loss"
)

// Service 处理换王：关闭旧蜂王记录并写入带标记色的新记录。
type Service struct {
	store *store.Memory
	clock func() time.Time
}

func New(memory *store.Memory, clock func() time.Time) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{store: memory, clock: clock}
}

type ReplaceInput struct {
	Origin       string
	IntroducedAt time.Time
	Reason       string
}

// succession 描述一次换王要关闭的旧蜂王与关闭方式。它在追加新记录之前
// 由当时读到的蜂箱快照推导，随后才交给仓储分别执行。
type succession struct {
	previousID string
	closeState domain.QueenState
	closedAt   time.Time
}

func planSuccession(
	hive domain.Hive,
	current domain.Queen,
	hasCurrent bool,
	reason string,
	at time.Time,
) succession {
	closeState := domain.QueenSuperseded
	if reason == ReasonLoss {
		closeState = domain.QueenLost
	}
	previousID := hive.ActiveQueenID
	if hasCurrent {
		previousID = current.ID
	}
	return succession{
		previousID: previousID,
		closeState: closeState,
		closedAt:   at.UTC(),
	}
}

func (s *Service) Replace(hiveID string, input ReplaceInput) (domain.Queen, error) {
	hive, ok := s.store.GetHive(hiveID)
	if !ok {
		return domain.Queen{}, domain.NotFound("蜂箱", hiveID)
	}
	current, hasCurrent := s.store.CurrentQueen(hiveID)
	origin, err := ParseOrigin(input.Origin)
	if err != nil {
		return domain.Queen{}, err
	}
	reason, err := ParseReason(input.Reason)
	if err != nil {
		return domain.Queen{}, err
	}
	introducedAt := input.IntroducedAt
	if introducedAt.IsZero() {
		introducedAt = s.clock()
	}
	record, err := domain.NewQueen(hiveID, introducedAt, origin, ColorForIntroduction(introducedAt))
	if err != nil {
		return domain.Queen{}, err
	}
	plan := planSuccession(hive, current, hasCurrent, reason, s.clock())
	saved, err := s.store.AppendQueen(hiveID, record)
	if err != nil {
		return domain.Queen{}, err
	}
	if plan.previousID != "" {
		if err := s.store.CloseQueen(hiveID, plan.previousID, plan.closeState, plan.closedAt); err != nil {
			return domain.Queen{}, err
		}
	}
	return saved, nil
}

func (s *Service) List(hiveID string) ([]domain.Queen, error) {
	snapshot := s.store.Snapshot()
	if _, ok := snapshot.Hives[hiveID]; !ok {
		return nil, domain.NotFound("蜂箱", hiveID)
	}
	records := snapshot.Queens(hiveID)
	if records == nil {
		records = []domain.Queen{}
	}
	return records, nil
}

func ParseOrigin(value string) (domain.QueenOrigin, error) {
	origin := domain.QueenOrigin(domain.TrimText(value))
	if !domain.ValidQueenOrigin(origin) {
		return "", domain.Invalid(
			"origin",
			"origin 必须是 %s、%s 或 %s",
			domain.OriginBred, domain.OriginCells, domain.OriginSwarm,
		)
	}
	return origin, nil
}

func ParseReason(value string) (string, error) {
	reason := domain.TrimText(value)
	switch reason {
	case ReasonAging, ReasonAggression, ReasonLoss:
		return reason, nil
	default:
		return "", domain.Invalid("reason", "reason 必须是 %s、%s 或 %s", ReasonAging, ReasonAggression, ReasonLoss)
	}
}
