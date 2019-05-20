#include "utils.h"
#include <string.h>

log_level_t log_level = WARN_LEVEL;
char *cid = NULL;
gboolean use_syslog = FALSE;
/* Set the log level for this call. log level defaults to warning.
   parse the string value of level_name to the appropriate log_level_t enum value
*/
void set_conmon_logs(char *level_name, char *cid_, gboolean syslog_)
{
	cid = cid_;
	use_syslog = syslog_;
	// log_level is initialized as Warning, no need to set anything
	if (level_name == NULL)
		return;
	if (!strcmp(level_name, "error") || !strcmp(level_name, "fatal") || !strcmp(level_name, "panic")) {
		log_level = EXIT_LEVEL;
		return;
	} else if (!strcmp(level_name, "warn") || !strcmp(level_name, "warning")) {
		log_level = WARN_LEVEL;
		return;
	} else if (!strcmp(level_name, "info")) {
		log_level = INFO_LEVEL;
		return;
	} else if (!strcmp(level_name, "debug")) {
		log_level = DEBUG_LEVEL;
		return;
	}
	nexitf("No such log level %s", level_name);
}
