// Package all регистрирует все семейства заданий: приложение импортирует только его.
//
//	import _ "fsp/pkg/itembank/families/all"
package all

import (
	_ "fsp/pkg/itembank/families/backend"
	_ "fsp/pkg/itembank/families/common"
	_ "fsp/pkg/itembank/families/dataml"
	_ "fsp/pkg/itembank/families/devops"
	_ "fsp/pkg/itembank/families/frontend"
	_ "fsp/pkg/itembank/families/qa"
	_ "fsp/pkg/itembank/families/security"
)
