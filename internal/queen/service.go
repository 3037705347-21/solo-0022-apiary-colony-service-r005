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

// closeStateFor 返回换王原因对应的旧蜂王关闭状态：失王登记为 lost，其余为被替换。
func closeStateFor(reason string) domain.QueenState {
	if reason == ReasonLoss {
		return domain.QueenLost
	}
	return domain.QueenSuperseded
}

func (s *Service) Replace(hiveID string, input ReplaceInput) (domain.Queen, error) {
	if _, ok := s.store.GetHive(hiveID); !ok {
		return domain.Queen{}, domain.NotFound("蜂箱", hiveID)
	}
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
	// 关闭旧王与追加新王在仓储的同一把写锁内原子完成，
	// 并发换王不会留下两只产卵蜂王，并发读取也看不到中间状态。
	return s.store.ReplaceQueen(hiveID, record, closeStateFor(reason), s.clock())
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
