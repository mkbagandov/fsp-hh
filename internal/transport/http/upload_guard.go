package httptransport

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2/adapters/humachi"

	"fsp/internal/domain"
	"fsp/internal/infrastructure/pdftext"
)

// resumeUploadPath — единственный маршрут с multipart-телом.
const resumeUploadPath = APIPrefix + "/candidate/resume"

// maxUploadBody — лимит тела загрузки: файл до pdftext.MaxBytes плюс служебные части multipart.
const maxUploadBody = pdftext.MaxBytes + 1<<20

func init() {
	// Тело ограничено maxUploadBody, поэтому разбираем его целиком в памяти: временные файлы
	// multipart на диске не создаются и не остаются после запроса (huma получает копию запроса,
	// и net/http их не удаляет).
	humachi.MultipartMaxMemory = maxUploadBody + 1<<20
}

// uploadGuard отсекает загрузку резюме до чтения тела: только аутентифицированный кандидат
// и не больше maxUploadBody байт. Huma разбирает multipart раньше, чем обработчик проверит роль,
// поэтому проверка нужна на уровне маршрутизатора.
func uploadGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != resumeUploadPath {
			next.ServeHTTP(w, r)
			return
		}
		if _, err := domain.RequireRole(r.Context(), domain.RoleCandidate); err != nil {
			status := http.StatusUnauthorized
			if domain.Is(err, domain.KindForbidden) {
				status = http.StatusForbidden
			}
			writeProblem(w, status, http.StatusText(status))
			return
		}
		if r.ContentLength > maxUploadBody {
			writeProblem(w, http.StatusRequestEntityTooLarge, "Файл больше 10 МБ")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadBody)
		next.ServeHTTP(w, r)
	})
}
