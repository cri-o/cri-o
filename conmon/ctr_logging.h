#pragma once
#if !defined(CTR_LOGGING_H)
#define CTR_LOGGING_H

#include "utils.h"   /* stdpipe_t */
#include <stdbool.h> /* bool */

void reopen_log_files(void);
bool write_to_logs(stdpipe_t pipe, char *buf, ssize_t num_read);
void configure_log_drivers(gchar **log_drivers, int64_t log_size_max_, char *cuuid_, char *name_);
void sync_logs(void);

#endif /* !defined(CTR_LOGGING_H) */
