#pragma once

#if !defined(CONFIG_H)
#define CONFIG_H

#define BUF_SIZE 8192
#define STDIO_BUF_SIZE 8192

/* stdpipe_t represents one of the std pipes (or NONE). */
typedef enum {
	NO_PIPE,
	STDIN_PIPE, /* unused */
	STDOUT_PIPE,
	STDERR_PIPE,
} stdpipe_t;


#endif // CONFIG_H
