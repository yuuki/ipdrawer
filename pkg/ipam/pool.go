package ipam

import (
	"net"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/taku-k/ipdrawer/pkg/model"
	"github.com/taku-k/ipdrawer/pkg/storage"
)

func setTSToPool(r *storage.Redis, pool *model.Pool) error {
	if err := pool.Validate(); err != nil {
		return err
	}

	if existsPool(r, pool) {
		stored, _ := getPool(r, net.ParseIP(pool.Start), net.ParseIP(pool.End))
		pool.CreatedAt = stored.CreatedAt
	}

	now := time.Now()
	if pool.CreatedAt == nil {
		pool.CreatedAt = &now
	} else {
		pool.LastModifiedAt = &now
	}

	return nil
}

func setPool(r *storage.Redis, pool *model.Pool) error {
	if err := pool.Validate(); err != nil {
		return err
	}

	pipe := r.Client.TxPipeline()
	s := net.ParseIP(pool.Start)
	e := net.ParseIP(pool.End)

	dkey := makePoolDetailsKey(s, e)
	data, err := pool.Marshal()
	if err != nil {
		return err
	}
	pipe.Set(dkey, string(data), 0)

	_, err = pipe.Exec()

	return err
}

func getPool(r *storage.Redis, start net.IP, end net.IP) (*model.Pool, error) {
	// Get details
	dkey := makePoolDetailsKey(start, end)

	check, err := r.Client.Exists(dkey).Result()
	if err != nil || check == 0 {
		return nil, errors.New("not found pool")
	}

	data, err := r.Client.Get(dkey).Result()
	if err != nil {
		return nil, err
	}
	pool := &model.Pool{}
	if err := pool.Unmarshal([]byte(data)); err != nil {
		return nil, err
	}

	return pool, nil
}

func getPools(r *storage.Redis, keys []string) ([]*model.Pool, error) {
	pools := make([]*model.Pool, len(keys))

	if len(keys) == 0 {
		return pools, nil
	}

	data, err := r.Client.MGet(keys...).Result()
	if err != nil {
		return nil, err
	}

	for i, d := range data {
		if s, ok := d.(string); ok {
			pools[i] = &model.Pool{}
			if err := pools[i].Unmarshal([]byte(s)); err != nil {
				return nil, err
			}
		}
	}
	return pools, nil
}

func getPoolsInNetwork(r *storage.Redis, prefix *model.Network) ([]*model.Pool, error) {
	_, pre, err := net.ParseCIDR(prefix.Prefix)
	if err != nil {
		return nil, err
	}
	poolKey := makeNetworkPoolKey(pre)
	keys, err := r.Client.SMembers(poolKey).Result()
	if err != nil {
		return nil, err
	}
	pools := make([]*model.Pool, len(keys))
	for i, key := range keys {
		start := net.ParseIP(key[:strings.Index(key, ",")])
		end := net.ParseIP(key[strings.Index(key, ",")+1:])
		pool, err := getPool(r, start, end)
		if err != nil {
			return nil, err
		}
		pools[i] = pool
	}
	return pools, nil
}

func existsPool(r *storage.Redis, pool *model.Pool) bool {
	s := net.ParseIP(pool.Start)
	e := net.ParseIP(pool.End)

	dkey := makePoolDetailsKey(s, e)
	check, _ := r.Client.Exists(dkey).Result()
	return check != 0
}
