//go:build darwin && desktop

package oscred

/*
#cgo LDFLAGS: -framework CoreFoundation -framework Security
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <stdlib.h>
#include <string.h>
static CFMutableDictionaryRef keyQuery(const char *id) {
 CFMutableDictionaryRef query=CFDictionaryCreateMutable(NULL,0,&kCFTypeDictionaryKeyCallBacks,&kCFTypeDictionaryValueCallBacks);
 CFStringRef account=CFStringCreateWithCString(NULL,id,kCFStringEncodingUTF8);
 CFDictionarySetValue(query,kSecClass,kSecClassGenericPassword);
 CFDictionarySetValue(query,kSecAttrService,CFSTR("com.lando.copilot-bridge.azure"));
 CFDictionarySetValue(query,kSecAttrAccount,account);CFRelease(account);return query;
}
static int sourceKeyRead(const char *id,void **result,long *length){
 CFMutableDictionaryRef query=keyQuery(id);CFDictionarySetValue(query,kSecReturnData,kCFBooleanTrue);
 CFDictionarySetValue(query,kSecMatchLimit,kSecMatchLimitOne);CFTypeRef data=NULL;
 OSStatus status=SecItemCopyMatching(query,&data);CFRelease(query);
 if(status!=errSecSuccess)return status;
 if(CFGetTypeID(data)!=CFDataGetTypeID()){CFRelease(data);return -1;}
 *length=CFDataGetLength((CFDataRef)data);*result=malloc(*length);
 if(*result)memcpy(*result,CFDataGetBytePtr((CFDataRef)data),*length);
 CFRelease(data);return *result?0:-1;
}
static int sourceKeySave(const char *id,const void *bytes,long length){
 CFMutableDictionaryRef query=keyQuery(id);CFDataRef data=CFDataCreate(NULL,bytes,length);
 CFMutableDictionaryRef update=CFDictionaryCreateMutable(NULL,0,&kCFTypeDictionaryKeyCallBacks,&kCFTypeDictionaryValueCallBacks);
 CFDictionarySetValue(update,kSecValueData,data);OSStatus status=SecItemUpdate(query,update);
 if(status==errSecItemNotFound){CFDictionarySetValue(query,kSecValueData,data);CFDictionarySetValue(query,kSecAttrAccessible,kSecAttrAccessibleWhenUnlockedThisDeviceOnly);status=SecItemAdd(query,NULL);}
 CFRelease(data);CFRelease(update);CFRelease(query);return status;
}
*/
import "C"
import (
	"fmt"
	"os"
	"unsafe"
)

func SourceKey(id string) (string, error) {
	name := C.CString(id)
	defer C.free(unsafe.Pointer(name))
	var data unsafe.Pointer
	var length C.long
	status := C.sourceKeyRead(name, &data, &length)
	if status == -25300 {
		return "", os.ErrNotExist
	}
	if status != 0 {
		return "", fmt.Errorf("Keychain could not read this source (%d)", int(status))
	}
	defer C.free(data)
	b := C.GoBytes(data, C.int(length))
	defer clear(b)
	return string(b), nil
}
func SaveSourceKey(id, key string) error {
	name := C.CString(id)
	defer C.free(unsafe.Pointer(name))
	data := C.CBytes([]byte(key))
	defer C.free(data)
	status := C.sourceKeySave(name, data, C.long(len(key)))
	if status != 0 {
		return fmt.Errorf("Keychain could not save this source (%d)", int(status))
	}
	return nil
}
