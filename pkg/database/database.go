package database

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"holosam/appengine/demo/pkg/util"

	"cloud.google.com/go/datastore"
	"google.golang.org/api/option"
)

var (
	includeFollowers = util.LoadEnvBool(util.EnvIncludeFollowers, false)
	txnRetryStrat    = util.LoadEnvString(util.EnvTxnRetryStrat, "none")
)

type DBClient struct {
	client *datastore.Client
	pool   *util.ThreadPool
	ctx    context.Context

	mu  sync.Mutex
	rnd *rand.Rand
}

func Init(ctx context.Context) (*DBClient, error) {
	opt := option.WithGRPCConnectionPool(util.LoadEnvInt(util.EnvConnPoolSize, 10))
	dbclient, err := datastore.NewClient(ctx, util.MustLoadEnvString(util.EnvCloudProject), opt)
	if err != nil {
		return nil, err
	}

	return &DBClient{
		client: dbclient,
		pool:   util.NewThreadPool(util.LoadEnvInt(util.EnvMaxThreads, 10)),
		ctx:    ctx,
		rnd:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}, err
}

func (d *DBClient) Close() {
	d.pool.Join(d.ctx)
	d.client.Close()
}

func (d *DBClient) ModifyUser(ctx context.Context, id string, modify func(u *User), create func() (User, error)) error {
	key := datastore.NameKey(userTable, id, nil)
	tries, waitTimeFunc := retryStrat(txnRetryStrat)
	for i := 0; i < tries; i++ {
		err := d.pool.RunSync(ctx, func() error {
			_, err := d.client.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
				var user User
				err := tx.Get(key, &user)
				if err == nil {
					modify(&user)
				} else if err == datastore.ErrNoSuchEntity {
					user, err = create()
					if err != nil {
						return err
					}
				} else {
					// Keep errors as is, to potentially catch them for retries.
					return err
				}

				_, err = tx.Put(key, &user)
				return err
			})
			return err
		})

		if err == nil {
			// No retries needed.
			break
		} else if err == datastore.ErrConcurrentTransaction {
			// Caller is supposed to retry this type of error.
			time.Sleep(waitTimeFunc(i))
		} else {
			// Probably a real issue, best to break here.
			return err
		}
	}

	return nil
}

func (d *DBClient) WriteDocument(ctx context.Context, pr *PublishRequest) error {
	var doc Document
	err := d.pool.RunSync(ctx, func() error {
		neededKeys := make([]*datastore.Key, 1)
		neededKeys[0] = datastore.IncompleteKey(docsTable, nil)
		allocKeys, err := d.client.AllocateIDs(ctx, neededKeys)
		if err != nil {
			return err
		}

		doc = Document{
			ID:          allocKeys[0].ID,
			Author:      pr.User,
			PublishTime: time.Now(),
			Text:        pr.Text,
		}

		key := datastore.IDKey(docsTable, doc.ID, nil)
		if _, err := d.client.Put(ctx, key, &doc); err != nil {
			return fmt.Errorf("db put doc error for key %v: %v", key, err)
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("write doc error: %v", err)
	}

	return d.ModifyUser(ctx, doc.Author, func(u *User) {
		u.AddDocument(doc.ID)
	}, ErrNoUser)
}

func (d *DBClient) GetUserDocs(ctx context.Context, id string, n int) ([]*Document, error) {
	user, err := d.getUser(ctx, id)
	if err != nil {
		return nil, err
	}

	if len(user.Documents) == 0 {
		return make([]*Document, 0), nil
	}

	if n > len(user.Documents) {
		n = len(user.Documents)
	}
	docs := make([]*Document, n)

	d.mu.Lock()
	// Would like to use a query for this, such as:
	// datastore.NewQuery(docsTable).Limit(n).Order("-PublishTime")
	// but there doesn't seem to be a way to use .Filter to match the keys.
	docKeys := make([]*datastore.Key, n)
	for i := 0; i < n; i++ {
		docID := user.Documents[d.rnd.Intn(len(user.Documents))]
		docKeys[i] = datastore.IDKey(docsTable, docID, nil)
	}
	d.mu.Unlock()

	err = d.pool.RunSync(ctx, func() error {
		return d.client.GetMulti(ctx, docKeys, docs)
	})

	return docs, err
}

func (d *DBClient) GetFollowingDocs(ctx context.Context, id string, n int) ([]*Document, error) {
	user, err := d.getUser(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get user error: %v", err)
	}

	if includeFollowers {
		user.Following = append(user.Following, user.Followers...)
	}

	if n > len(user.Following) {
		n = len(user.Following)
	}

	d.mu.Lock()
	dstIndices := make([]int, n)
	for i := 0; i < n; i++ {
		dstIndices[i] = d.rnd.Intn(len(user.Following))
	}
	d.mu.Unlock()

	feedDocs := make([]*Document, 0)
	err = d.pool.RunSync(ctx, func() error {
		for _, dstIndex := range dstIndices {
			dst := user.Following[dstIndex]
			dstDocs, err := d.GetUserDocs(ctx, dst, 1)
			if err != nil {
				return fmt.Errorf("user docs error: %v", err)
			}
			feedDocs = append(feedDocs, dstDocs...)
		}
		return nil
	})

	return feedDocs, err
}

func (d *DBClient) getUser(ctx context.Context, id string) (*User, error) {
	var user User
	err := d.pool.RunSync(ctx, func() error {
		key := datastore.NameKey(userTable, id, nil)
		return d.client.Get(ctx, key, &user)
	})

	return &user, err
}

func ErrNoUser() (User, error) {
	return User{}, fmt.Errorf("user should already exist")
}

func retryStrat(strat string) (int, func(int) time.Duration) {
	switch strat {
	case "three":
		return 3, func(i int) time.Duration {
			return 100 * time.Millisecond
		}
	case "exp_backoff":
		return 4, func(i int) time.Duration {
			i += 1
			return time.Millisecond * time.Duration(10*i*i)
		}
	default: // default is not retrying at all.
		return 1, func(i int) time.Duration {
			return time.Millisecond
		}
	}
}
