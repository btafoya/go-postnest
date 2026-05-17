package mailstore

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/go-postnest/postnest/internal/models"
)

// ListOptions controls pagination and sorting.
type ListOptions struct {
	Limit     int
	Offset    int
	SortField string
	SortDesc  bool
}


// SearchOptions provides additional search filters.
type SearchOptions struct {
	LabelID       *uuid.UUID
	From          string
	To            string
	HasAttachment bool
	DateAfter     *time.Time
	DateBefore    *time.Time
	Limit         int
	Offset        int
}

// MessagePatch contains optional fields for message updates.
type MessagePatch struct {
	IsRead      *bool
	IsFlagged   *bool
	IsAnswered  *bool
	IsDraft     *bool
	Mailbox     *string
	IsOutbound  *bool
	Subject     *string
	HTMLBody    *string
	PlainText   *string
	ToAddresses []string
}

// Store is the canonical interface for mail persistence.
type Store interface {
	CreateMessage(ctx context.Context, msg *models.Message, labelIDs []uuid.UUID, attachments []*models.Attachment) error
	GetMessage(ctx context.Context, domainID, userID, messageID uuid.UUID) (*models.Message, error)
	ListMessages(ctx context.Context, domainID, userID uuid.UUID, labelID *uuid.UUID, opts ListOptions) ([]*models.Message, int64, error)
	UpdateMessage(ctx context.Context, domainID, userID, messageID uuid.UUID, patch MessagePatch) error
	DeleteMessage(ctx context.Context, domainID, userID, messageID uuid.UUID) error
	MoveToMailbox(ctx context.Context, domainID, userID, messageID uuid.UUID, mailbox string) error

	CreateLabel(ctx context.Context, label *models.Label) error
	GetLabels(ctx context.Context, domainID, userID uuid.UUID) ([]*models.Label, error)
	GetLabelByName(ctx context.Context, domainID, userID uuid.UUID, name string) (*models.Label, error)
	DeleteLabel(ctx context.Context, domainID, userID, labelID uuid.UUID) error

	ApplyLabels(ctx context.Context, messageID uuid.UUID, addLabelIDs, removeLabelIDs []uuid.UUID) error
	GetMessageLabels(ctx context.Context, messageID uuid.UUID) ([]*models.Label, error)

	GetThread(ctx context.Context, domainID, userID, threadID uuid.UUID) (*models.Thread, []*models.Message, error)
	FindOrCreateThread(ctx context.Context, domainID, userID uuid.UUID, subject, messageID, inReplyTo string, references []string) (*models.Thread, error)

	CreateAttachments(ctx context.Context, attachments []*models.Attachment) error
	GetAttachment(ctx context.Context, attachmentID uuid.UUID) (*models.Attachment, error)

	SetFlag(ctx context.Context, messageID uuid.UUID, flag string) error
	ClearFlag(ctx context.Context, messageID uuid.UUID, flag string) error
	GetFlags(ctx context.Context, messageID uuid.UUID) ([]string, error)
	GetFlagsBatch(ctx context.Context, messageIDs []uuid.UUID) (map[uuid.UUID][]string, error)

	UpdateLabel(ctx context.Context, domainID, userID, labelID uuid.UUID, name, color string) error

	Search(ctx context.Context, domainID, userID uuid.UUID, query string, opts SearchOptions) ([]*models.Message, int64, error)
	UpdateSearchVector(ctx context.Context, messageID uuid.UUID) error

	CountUnreadByLabel(ctx context.Context, domainID, userID uuid.UUID, labelID uuid.UUID) (int64, error)
	CountTotalByLabel(ctx context.Context, domainID, userID uuid.UUID, labelID uuid.UUID) (int64, error)

	CreateDeliveryLog(ctx context.Context, log *models.DeliveryLog) error
}
