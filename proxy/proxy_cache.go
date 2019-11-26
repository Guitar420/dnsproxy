package proxy

import (
	"net"

	"github.com/AdguardTeam/golibs/log"
	"github.com/miekg/dns"
)

// Get response from general or subnet cache
// Return TRUE if response is found in cache
func (p *Proxy) replyFromCache(d *DNSContext) bool {
	if p.cache == nil {
		return false
	}

	if !p.Config.EnableEDNSClientSubnet {
		val, ok := p.cache.Get(d.Req)
		if ok && val != nil {
			d.Res = val
			log.Tracef("Serving cached response")
			return true
		}
		return false
	}

	d.ecsReqIP = nil
	d.ecsReqMask = uint8(0)

	ip, mask, _ := parseECS(d.Req)
	if mask == 0 {
		// Set EDNS Client-Subnet data
		var clientIP net.IP
		switch addr := d.Addr.(type) {
		case *net.UDPAddr:
			clientIP = addr.IP
		case *net.TCPAddr:
			clientIP = addr.IP
		}

		if clientIP != nil && isPublicIP(clientIP) {
			ip, mask = setECS(d.Req, clientIP, 0)
			log.Debug("Set ECS data: %s/%d", ip, mask)
		}
	} else {
		log.Debug("Passing through ECS data: %s/%d", ip, mask)
	}

	if mask != 0 && p.cacheSubnet != nil {
		val, ok := p.cacheSubnet.GetWithSubnet(d.Req, ip, mask)
		if ok && val != nil {
			d.Res = val
			log.Debug("Serving response from subnet cache")
			return true
		}
	} else if mask == 0 && p.cache != nil {
		val, ok := p.cache.Get(d.Req)
		if ok && val != nil {
			d.Res = val
			log.Debug("Serving response from general cache")
			return true
		}
	}

	d.ecsReqIP = ip
	d.ecsReqMask = mask
	return false
}

// Store response in general or subnet cache
func (p *Proxy) setInCache(d *DNSContext, resp *dns.Msg) {
	if p.cache == nil {
		return
	}

	if !p.Config.EnableEDNSClientSubnet {
		p.cache.Set(resp)
		return
	}

	ip, mask, scope := parseECS(resp)
	if ip != nil {
		if ip.Equal(d.ecsReqIP) && mask == d.ecsReqMask {
			log.Debug("ECS option in response: %s/%d", ip, scope)
			p.cacheSubnet.SetWithSubnet(resp, ip, scope)
		} else {
			log.Debug("Invalid response from server: ECS data mismatch: %s/%d -- %s/%d",
				d.ecsReqIP, d.ecsReqMask, ip, mask)
		}
	} else if d.ecsReqIP != nil {
		// server doesn't support ECS - cache response for all subnets
		p.cacheSubnet.SetWithSubnet(resp, ip, scope)
	} else {
		p.cache.Set(resp) // use general cache
	}
}
