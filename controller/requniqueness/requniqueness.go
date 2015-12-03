package requniqueness

import (
	"errors"
	"sync"
)

type reqHandleCache struct {
	locker   sync.RWMutex
	ReqCache map[string]bool
}

var (
	handleCache = map[string]*reqHandleCache{}
	gLocker     sync.RWMutex
)

var (
	Error_HandleAlready = errors.New("HandleAlready")
)

//成功时返回nil，失败时返回Error_HandleAlready（表示已经被handle了）
func Handle(reqType string, subKey string) error {
	gLocker.Lock()
	if _, ok := handleCache[reqType]; ok == false {
		handleCache[reqType] = &reqHandleCache{
			ReqCache: map[string]bool{},
		}
	}
	gLocker.Unlock()

	handleCache[reqType].locker.Lock()
	defer handleCache[reqType].locker.Unlock()

	_, ok := handleCache[reqType].ReqCache[subKey]
	if ok == true {
		return Error_HandleAlready
	}
	handleCache[reqType].ReqCache[subKey] = true
	return nil
}

func Release(reqType string, subKey string) {
	handleCache[reqType].locker.Lock()
	delete(handleCache[reqType].ReqCache, subKey) //将subKey移除
	handleCache[reqType].locker.Unlock()
	return
}
