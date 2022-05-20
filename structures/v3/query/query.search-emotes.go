package query

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/SevenTV/Common/errors"
	"github.com/SevenTV/Common/mongo"
	"github.com/SevenTV/Common/structures/v3"
	"github.com/SevenTV/Common/utils"
	"github.com/meilisearch/meilisearch-go"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const EMOTES_QUERY_LIMIT = 300

type SearchHit struct {
	ID           primitive.ObjectID `json:"id"`
	Name         string             `json:"name"`
	Tags         []string           `json:"tags"`
	OwnerID      primitive.ObjectID `json:"owner_id"`
	Listed       bool               `json:"listed"`
	ChannelCount int                `json:"channel_count"`
	CreatedAt    int                `json:"created_at"`
}

func (q *Query) SearchEmotes(ctx context.Context, opt SearchEmotesOptions) ([]structures.Emote, int, error) {
	// Define the query string
	query := strings.TrimSpace(opt.Query)

	client := meilisearch.NewClient(meilisearch.ClientConfig{
		Host:   opt.Meilisearch.Host,
		APIKey: opt.Meilisearch.ApiKey,
	})

	filter := []string{}
	if opt.Filter != "" {
		filter = append(filter, opt.Filter)
	}

	// Apply permission checks
	// omit unlisted/private emotes
	if !opt.ShowHidden {
		filter = append(filter, "listed = true")
	}

	var finalFilter *string
	if len(filter) != 0 {
		finalFilter = utils.PointerOf("(" + strings.Join(filter, ") and (") + ")")
	}

	resp, err := client.Index("emotes").Search(query, &meilisearch.SearchRequest{
		Offset:            int64((opt.Page - 1) * opt.Limit),
		Limit:             int64(opt.Limit),
		Sort:              opt.Sort,
		PlaceholderSearch: query == "",
		Filter:            finalFilter,
	})
	if err != nil {
		return nil, 0, err
	}

	result := []structures.Emote{}
	if len(resp.Hits) != 0 {
		hits := []SearchHit{}
		rawHits, _ := json.Marshal(resp.Hits)
		err := json.Unmarshal(rawHits, &hits)
		if err != nil {
			return nil, 0, err
		}

		ids := make([]primitive.ObjectID, len(hits))
		for i, hit := range hits {
			ids[i] = hit.ID
		}

		cur, err := q.mongo.Collection(mongo.CollectionNameEmotes).Find(ctx, bson.M{
			"_id": bson.M{
				"$in": ids,
			},
		})
		if err != nil {
			return nil, 0, errors.ErrInternalServerError().SetDetail(err.Error())
		}
		if err = cur.All(ctx, &result); err != nil {
			return nil, 0, err
		}
	}

	// Paginate and fetch the relevant emotes
	return result, int(resp.NbHits), nil
}

type SearchEmotesOptions struct {
	Query       string
	Page        int
	Limit       int
	Sort        []string
	Filter      string
	ShowHidden  bool
	Meilisearch SearchEmotesOptionsMeilisearch
}

type SearchEmotesOptionsMeilisearch struct {
	Host   string
	ApiKey string
}
