package ipam

import (
	"fmt"
	"net"
	"time"

	"github.com/go-redis/redis"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"golang.org/x/net/context"

	"github.com/taku-k/ipdrawer/pkg/base"
	"github.com/taku-k/ipdrawer/pkg/model"
	"github.com/taku-k/ipdrawer/pkg/storage"
	"github.com/taku-k/ipdrawer/pkg/utils/netutil"
)

type IPManager struct {
	redis  *storage.Redis
	locker storage.Locker
}

// NewIPManager creates IPManager instance
func NewIPManager(cfg *base.Config) *IPManager {
	redis, err := storage.NewRedis(cfg)
	if err != nil {
		panic(errors.New(fmt.Sprintf("Connection is faied with redis: %#+v", err)))
	}
	locker := storage.NewLocker(redis)
	return &IPManager{
		redis:  redis,
		locker: locker,
	}
}

// DrawIP returns an available IP.
func (m *IPManager) DrawIP(ctx context.Context, pool *model.Pool, reserve bool, ping bool) (net.IP, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "IPManager.DrawIP")
	span.SetTag("pool", pool.Key())
	defer span.Finish()

	token, err := m.locker.Lock(ctx, makeGlobalLock())
	if err != nil {
		return nil, err
	}
	defer m.locker.Unlock(ctx, makeGlobalLock(), token)

	s := net.ParseIP(pool.Start)
	e := net.ParseIP(pool.End)
	zkey := makePoolUsedIPZset(s, e)
	avail := s

	keys, err := m.redis.Client.ZRange(zkey, 0, -1).Result()
	if err != nil {
		return nil, err
	}

	i := 0
	for !netutil.PrevIP(avail).Equal(e) {
		flag := false
		if i < len(keys) {
			usedIP := net.ParseIP(keys[i])
			if usedIP != nil {
				if avail.Equal(usedIP) {
					flag = true
					i += 1
				}
			}
		}
		if !flag {
			check, err := m.redis.Client.Exists(makeIPTempReserved(avail)).Result()
			if err != nil || check != 0 {
				flag = true

			} else {
				if _, err = m.redis.Client.Set(makeIPTempReserved(avail), 1, 24*time.Hour).Result(); err != nil {
					flag = true
				}
			}
		}
		if !flag {
			return avail, nil
		} else {
			avail = netutil.NextIP(avail)
		}
	}
	return nil, errors.New("Nothing IP to serve")
}

// CreateIP activates IP.
func (m *IPManager) CreateIP(ctx context.Context, ps []*model.Pool, addr *model.IPAddr) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "IPManager.Activate")
	span.SetTag("ip", addr.Ip)
	defer span.Finish()

	token, err := m.locker.Lock(ctx, makeGlobalLock())
	if err != nil {
		return err
	}
	defer m.locker.Unlock(ctx, makeGlobalLock(), token)

	ip := net.ParseIP(addr.Ip)

	if err := setTSToIPAddr(m.redis, addr); err != nil {
		return err
	}

	if err := setIPAddr(m.redis, addr); err != nil {
		return err
	}

	pipe := m.redis.Client.TxPipeline()
	// Remove temporary reserved key in any way
	pipe.Del(makeIPTempReserved(ip))
	// Add IP to used IP zset
	score := float64(netutil.IP2Uint(ip))
	z := redis.Z{
		Score:  score,
		Member: ip.String(),
	}
	for _, p := range ps {
		if p.Contains(ip) {
			s := net.ParseIP(p.Start)
			e := net.ParseIP(p.End)
			pipe.ZAdd(makePoolUsedIPZset(s, e), z)
		}
	}
	if _, err := pipe.Exec(); err != nil {
		return err
	}
	return nil
}

func (m *IPManager) Deactivate(ctx context.Context, ps []*model.Pool, addr *model.IPAddr) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "IPManager.Deactivate")
	span.SetTag("pool", ps[0].Key())
	span.SetTag("ip", addr.Ip)
	defer span.Finish()

	token, err := m.locker.Lock(ctx, makeGlobalLock())
	if err != nil {
		return err
	}
	defer m.locker.Unlock(ctx, makeGlobalLock(), token)

	ip := net.ParseIP(addr.Ip)

	pipe := m.redis.Client.TxPipeline()
	pipe.Del(makeIPTempReserved(ip))
	pipe.Del(makeIPDetailsKey(ip))
	for _, p := range ps {
		if p.Contains(ip) {
			s := net.ParseIP(p.Start)
			e := net.ParseIP(p.End)
			pipe.ZRem(makePoolUsedIPZset(s, e), ip.String())
		}
	}
	if _, err := pipe.Exec(); err != nil {
		return err
	}
	return nil
}

func (m *IPManager) UpdateIP(ctx context.Context, addr *model.IPAddr) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "IPManager.UpdateIP")
	span.SetTag("ip", addr.Ip)
	defer span.Finish()

	token, err := m.locker.Lock(ctx, makeGlobalLock())
	if err != nil {
		return err
	}
	defer m.locker.Unlock(ctx, makeGlobalLock(), token)

	if !existsIP(m.redis, addr) {
		return errors.New("Not found IP")
	}

	if err := setTSToIPAddr(m.redis, addr); err != nil {
		return err
	}

	return setIPAddr(m.redis, addr)
}

// Reserve makes the status of given IP reserved.
func (m *IPManager) Reserve(p *model.Pool, ip net.IP) error {
	return nil
}

// Release makes the status of given IP available.
func (m *IPManager) Release(p *model.Pool, ip net.IP) error {
	return nil
}

// GetNetworkIncludingIP returns a network including given IP.
func (m *IPManager) GetNetworkIncludingIP(ctx context.Context, ip net.IP) (*model.Network, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "IPManager.GetNetworkIncludingIP")
	span.SetTag("ip", ip.String())
	defer span.Finish()

	ps, err := m.redis.Client.SMembers(makeNetworkListKey()).Result()
	if err != nil {
		return nil, err
	}
	for _, p := range ps {
		_, ipnet, err := net.ParseCIDR(p)
		if err != nil {
			continue
		}
		if ipnet.Contains(ip) {
			net, err := getNetwork(m.redis, ipnet)
			return net, err
		}
	}
	return nil, errors.New(fmt.Sprintf("Not found network including the IP: %s", ip.String()))
}

// GetPoolsInNetwork gets pools.
func (m *IPManager) GetPoolsInNetwork(ctx context.Context, n *model.Network) ([]*model.Pool, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "IPManager.GetPoolsInNetwork")
	span.SetTag("network", n.String())
	defer span.Finish()

	return getPoolsInNetwork(m.redis, n)
}

// GetNetworkByIP returns network by IP.
func (m *IPManager) GetNetworkByIP(ctx context.Context, ipnet *net.IPNet) (*model.Network, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "IPManager.GetNetworkByIP")
	span.SetTag("ip", ipnet.String())
	defer span.Finish()

	return getNetwork(m.redis, ipnet)
}

// GetNetworkByName returns network by name.
func (m *IPManager) GetNetworkByName(ctx context.Context, name string) (*model.Network, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "IPManager.GetNetworkByName")
	span.SetTag("name", name)
	defer span.Finish()

	networks, err := getNetworks(m.redis)
	if err != nil {
		return nil, err
	}

	target := &model.Tag{
		Key:   "Name",
		Value: name,
	}
	for _, n := range networks {
		if n.HasTag(target) {
			return n, nil
		}
	}

	return nil, errors.New("Not found network")
}

// CreateNetwork creates network.
func (m *IPManager) CreateNetwork(ctx context.Context, n *model.Network) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "IPManager.CreateNetwork")
	span.SetTag("network", n.String())
	defer span.Finish()

	token, err := m.locker.Lock(ctx, makeGlobalLock())
	if err != nil {
		return err
	}
	defer m.locker.Unlock(ctx, makeGlobalLock(), token)

	if err := setTSToNetwork(m.redis, n); err != nil {
		return err
	}

	return setNetwork(m.redis, n)
}

// DeleteNetwork deletes the network
func (m *IPManager) DeleteNetwork(ctx context.Context, n *model.Network) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "IPManager.DeleteNetwork")
	span.SetTag("network", n.String())
	defer span.Finish()

	token, err := m.locker.Lock(ctx, makeGlobalLock())
	if err != nil {
		return err
	}
	defer m.locker.Unlock(ctx, makeGlobalLock(), token)

	return deleteNetwork(m.redis, n)
}

// UpdateNetwork updates the network
func (m *IPManager) UpdateNetwork(ctx context.Context, n *model.Network) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "IPManager.UpdateNetwork")
	span.SetTag("network", n.String())
	defer span.Finish()

	token, err := m.locker.Lock(ctx, makeGlobalLock())
	if err != nil {
		return err
	}
	defer m.locker.Unlock(ctx, makeGlobalLock(), token)

	if !existsNetwork(m.redis, n) {
		return errors.New("Not found Network")
	}

	if err := setTSToNetwork(m.redis, n); err != nil {
		return err
	}

	return setNetwork(m.redis, n)
}

// CreatePool creates pool
func (m *IPManager) CreatePool(ctx context.Context, n *model.Network, pool *model.Pool) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "IPManager.CreatePool")
	span.SetTag("network", n.String())
	span.SetTag("pool", pool.Key())
	defer span.Finish()

	if err := addPoolToNetwork(m.redis, n, pool); err != nil {
		return err
	}

	// Get IPs in this pool to add these to a used zset.
	addrs, err := m.ListIP(ctx)
	if err != nil {
		return err
	}

	s := net.ParseIP(pool.Start)
	e := net.ParseIP(pool.End)
	usedkey := makePoolUsedIPZset(s, e)
	members := make([]redis.Z, 0, len(addrs))
	for _, _ip := range addrs {
		ip := net.ParseIP(_ip.Ip)
		if !pool.Contains(ip) {
			continue
		}

		score := float64(netutil.IP2Uint(ip))
		z := redis.Z{
			Score:  score,
			Member: ip.String(),
		}
		members = append(members, z)
	}
	if len(members) != 0 {
		_, err = m.redis.Client.ZAdd(usedkey, members...).Result()
		if err != nil {
			return err
		}
	}

	if err := setTSToPool(m.redis, pool); err != nil {
		return err
	}

	return setPool(m.redis, pool)
}

func (m *IPManager) ListIP(ctx context.Context) ([]*model.IPAddr, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "IPManager.ListIP")
	defer span.Finish()

	keys, err := m.redis.Client.Keys(makeIPListPattern()).Result()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to fetch IP list keys")
	}

	ips := make([]net.IP, len(keys))
	for i, key := range keys {
		ip, err := parseIPDetailsKey(key)
		if err != nil {
			return nil, errors.Wrapf(err, "Parse failed: %s", key)
		}
		ips[i] = ip
	}

	return getIPAddrs(m.redis, ips)
}

// GetTemporaryReservedIPs returns all temporary reserved ips.
func (m *IPManager) GetTemporaryReservedIPs(ctx context.Context) ([]*model.IPAddr, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "IPManager.GetTemporaryReservedIPs")
	defer span.Finish()

	keys, err := m.redis.Client.Keys(makeTempReservedIPListPattern()).Result()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to fetch IP list keys")
	}

	addrs := make([]*model.IPAddr, len(keys))
	for i, key := range keys {
		ip, err := parseTempReservedIPKey(key)
		if err != nil {
			return nil, errors.Wrapf(err, "Parse failed: %s", key)
		}
		addrs[i] = &model.IPAddr{
			Ip:     ip.String(),
			Status: model.IPAddr_TEMPORARY_RESERVED,
		}
	}
	return addrs, nil
}

// GetNetworks returns all network.
func (m *IPManager) GetNetworks(ctx context.Context) ([]*model.Network, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "IPManager.GetNetworks")
	defer span.Finish()

	return getNetworks(m.redis)
}

// GetPools returns all pools.
func (m *IPManager) GetPools(ctx context.Context) ([]*model.Pool, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "IPManager.GetPools")
	defer span.Finish()

	keys, err := m.redis.Client.Keys(makePoolListPattern()).Result()
	if err != nil {
		return nil, errors.Wrap(err, "Failed fetch Pool list keys")
	}

	return getPools(m.redis, keys)
}

func (m *IPManager) GetPool(ctx context.Context, s net.IP, e net.IP) (*model.Pool, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "IPManager.GetPool")
	defer span.Finish()

	return getPool(m.redis, s, e)
}

func (m *IPManager) UpdatePool(ctx context.Context, pool *model.Pool) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "IPManager.UpdatePool")
	span.SetTag("pool", pool.Key())
	defer span.Finish()

	token, err := m.locker.Lock(ctx, makeGlobalLock())
	if err != nil {
		return err
	}
	defer m.locker.Unlock(ctx, makeGlobalLock(), token)

	if !existsPool(m.redis, pool) {
		return errors.New("Not found Pool")
	}

	if err := setTSToPool(m.redis, pool); err != nil {
		return err
	}

	return setPool(m.redis, pool)
}

func (m *IPManager) DeletePool(ctx context.Context, start, end net.IP) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "IPManager.DeletePool")
	defer span.Finish()

	token, err := m.locker.Lock(ctx, makeGlobalLock())
	if err != nil {
		return err
	}
	defer m.locker.Unlock(ctx, makeGlobalLock(), token)

	pipe := m.redis.Client.TxPipeline()
	pipe.Del(makePoolDetailsKey(start, end))
	pipe.Del(makePoolUsedIPZset(start, end))
	_, err = pipe.Exec()

	return err
}
