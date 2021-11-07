package dao

import (
	"crypto/md5" // #nosec
	"encoding/hex"
	"log"
	"sync"
	"time"

	"github.com/naiba/nezha/model"
)

const firstNotificationDelay = time.Minute * 15

// 通知方式
var notifications notification

type notification struct {
	nty  []model.Notification
	lock sync.RWMutex
}

func LoadNotifications() {
	notifications.lock.Lock()
	if err := DB.Find(&notifications).Error; err != nil {
		panic(err)
	}
	notifications.lock.Unlock()
}

func OnRefreshOrAddNotification(n model.Notification) {
	notifications.lock.Lock()
	defer notifications.lock.Unlock()
	var isEdit bool
	for i := 0; i < len(notifications.nty); i++ {
		if notifications.nty[i].ID == n.ID {
			notifications.nty[i] = n
			isEdit = true
		}
	}
	if !isEdit {
		notifications.nty = append(notifications.nty, n)
	}
}

func OnDeleteNotification(id uint64) {
	notifications.lock.Lock()
	defer notifications.lock.Unlock()
	for i := 0; i < len(notifications.nty); i++ {
		if notifications.nty[i].ID == id {
			notifications.nty = append(notifications.nty[:i], notifications.nty[i+1:]...)
			i--
		}
	}
}

func SendNotification(desc string, muteable bool) {
	if muteable {
		// 通知防骚扰策略
		nID := hex.EncodeToString(md5.New().Sum([]byte(desc))) // #nosec
		var flag bool
		if cacheN, has := Cache.Get(nID); has {
			nHistory := cacheN.(NotificationHistory)
			// 每次提醒都增加一倍等待时间，最后每天最多提醒一次
			if time.Now().After(nHistory.Until) {
				flag = true
				nHistory.Duration *= 2
				if nHistory.Duration > time.Hour*24 {
					nHistory.Duration = time.Hour * 24
				}
				nHistory.Until = time.Now().Add(nHistory.Duration)
				// 缓存有效期加 10 分钟
				Cache.Set(nID, nHistory, nHistory.Duration+time.Minute*10)
			}
		} else {
			// 新提醒直接通知
			flag = true
			Cache.Set(nID, NotificationHistory{
				Duration: firstNotificationDelay,
				Until:    time.Now().Add(firstNotificationDelay),
			}, firstNotificationDelay+time.Minute*10)
		}

		if !flag {
			if Conf.Debug {
				log.Println("NEZHA>> 静音的重复通知：", desc, muteable)
			}
			return
		}
	}
	// 发出通知
	notifications.lock.RLock()
	defer notifications.lock.RUnlock()
	for i := 0; i < len(notifications.nty); i++ {
		if err := notifications.nty[i].Send(desc); err != nil {
			log.Println("NEZHA>> 发送通知失败：", err)
		}
	}
}
