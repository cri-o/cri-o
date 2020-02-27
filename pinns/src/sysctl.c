#define _GNU_SOURCE

#include <glib.h>
#include <string.h>
#include <fcntl.h>

#include "sysctl.h"
#include "utils.h"

static int separate_sysctl_key_value (char* sysctl_key_value, char** sysctl_key, char** sysctl_value);
static int write_sysctl_to_file (char * sysctl_key, char* sysctl_value);

int configure_sysctls (const GPtrArray *sysctls) {
  size_t n_sysctls = 0;
  while (g_ptr_array_index (sysctls, n_sysctls))
  {
    char* key = NULL;
    char* value = NULL;
    if (separate_sysctl_key_value (g_ptr_array_index (sysctls, n_sysctls++), &key, &value) < 0)
      return -1;

    if (write_sysctl_to_file(key, value) < 0)
      return -1;
  }

  return 0;
}

// sysctl_key_value should be in the form `'key=value'`
static int separate_sysctl_key_value (char* sysctl_key_value, char** sysctl_key, char** sysctl_value)
{
  // begin by stripping the `'`, we now have `key=value'`
  if (*sysctl_key_value == '\'') {
    sysctl_key_value++;
  }

  // now find the `=` and convert it to a delimiter
  char * equals_token = strchr (sysctl_key_value, '=');
  if (!equals_token) {
    pwarnf("sysctl must be in the form of 'key=value'; '=' missing");
    return -1;
  }
  // we now have `key\0value'`
  *equals_token = '\0';

  // sysctl_key is now key
  *sysctl_key = sysctl_key_value;

  // equals_token is now value'
  ++equals_token;

  // if sysctl ends in a ', we should strip it
  char* ending_char = strchr(equals_token, '\'');
  *ending_char = '\0';

  // sysctl_value is now value
  *sysctl_value = equals_token;

  if (!strlen (*sysctl_key))
  {
    pwarnf ("sysctl must be in the form of 'key=value'; key is empty");
    return -1;
  }
  if (!strlen (*sysctl_value))
  {
    pwarnf ("sysctl must be in the form of 'key=value'; value is empty");
    return -1;
  }
  return 0;
}

static int write_sysctl_to_file (char * sysctl_key, char* sysctl_value)
{
  if (!sysctl_key || !sysctl_value)
  {
    pwarnf ("sysctl key or value not initialized");
    return -1;
  }

  // replace periods with / to create the sysctl path
  for (char* it = sysctl_key; *it; it++)
    if (*it == '.')
      *it = '/';

  _cleanup_close_ int dirfd = open ("/proc/sys", O_DIRECTORY | O_RDONLY);
  if (UNLIKELY (dirfd < 0)) {
    pwarnf ("failed to open /proc/sys");
    return -1;
  }

  _cleanup_close_ int fd = openat (dirfd, sysctl_key, O_WRONLY);
  if (UNLIKELY (fd < 0)) {
    pwarnf("failed to open /proc/sys/%s", sysctl_key);
    return -1;
  }

  int ret = TEMP_FAILURE_RETRY (write (fd, sysctl_value, strlen (sysctl_value)));
  if (UNLIKELY (ret < 0)) {
    pwarnf("failed to write to /proc/sys/%s", sysctl_key);
    return -1;
  }
  return 0;
}
