#pragma once
#if !defined(UTILS_H)
#define UTILS_H

#include <errno.h>
#include <stdbool.h>
#include <stdio.h>
#include <string.h>
#include <stdlib.h>
#include <sys/uio.h>
#include <syslog.h>
#include <unistd.h>

#ifndef TEMP_FAILURE_RETRY
#define TEMP_FAILURE_RETRY(expression)                                         \
  (__extension__({                                                             \
    long int __result;                                                         \
    do                                                                         \
      __result = (long int)(expression);                                       \
    while (__result == -1L && errno == EINTR);                                 \
    __result;                                                                  \
  }))
#endif

#define _pexit(s)                                                              \
  do {                                                                         \
    fprintf(stderr, "[pinns:e]: %s: %s\n", s, strerror(errno));                \
    _exit(EXIT_FAILURE);                                                       \
  } while (0)

#define pexit(s)                                                               \
  do {                                                                         \
    fprintf(stderr, "[pinns:e]: %s: %s\n", s, strerror(errno));                \
    exit(EXIT_FAILURE);                                                        \
  } while (0)

#define pexitf(fmt, ...)                                                       \
  do {                                                                         \
    fprintf(stderr, "[pinns:e]: " fmt ": %s\n", ##__VA_ARGS__,                 \
            strerror(errno));                                                  \
    exit(EXIT_FAILURE);                                                        \
  } while (0)

#define pwarn(s)                                                               \
  do {                                                                         \
    fprintf(stderr, "[pinns:w]: %s: %s\n", s, strerror(errno));                \
  } while (0)

#define pwarnf(fmt, ...)                                                       \
  do {                                                                         \
    fprintf(stderr, "[pinns:w]: " fmt ": %s\n", ##__VA_ARGS__,                 \
            strerror(errno));                                                  \
  } while (0)

#define nexit(s)                                                               \
  do {                                                                         \
    fprintf(stderr, "[pinns:e] %s\n", s);                                      \
    exit(EXIT_FAILURE);                                                        \
  } while (0)

#define nexitf(fmt, ...)                                                       \
  do {                                                                         \
    fprintf(stderr, "[pinns:e]: " fmt "\n", ##__VA_ARGS__);                    \
    exit(EXIT_FAILURE);                                                        \
  } while (0)

#define nwarn(s)                                                               \
  do {                                                                         \
    fprintf(stderr, "[pinns:w]: %s\n", s);                                     \
  } while (0);

#define nwarnf(fmt, ...)                                                       \
  do {                                                                         \
    fprintf(stderr, "[pinns:w]: " fmt "\n", ##__VA_ARGS__);                    \
  } while (0);

#define _cleanup_(x) __attribute__((cleanup(x)))

static inline void freep(void *p) { free(*(void **)p); }

static inline void closep(int *fd) {
  if (*fd >= 0)
    close(*fd);
  *fd = -1;
}

static inline void fclosep(FILE **fp) {
  if (*fp)
    fclose(*fp);
  *fp = NULL;
}

#define _cleanup_free_ _cleanup_(freep)
#define _cleanup_close_ _cleanup_(closep)
#define _cleanup_fclose_ _cleanup_(fclosep)

# define LIKELY(x) __builtin_expect((x),1)
# define UNLIKELY(x) __builtin_expect((x),0)

#endif /* !defined(UTILS_H) */
