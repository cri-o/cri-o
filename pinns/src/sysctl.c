#define _GNU_SOURCE

#include <string.h>
#include <fcntl.h>

#include "sysctl.h"
#include "utils.h"

static int separate_sysctl_key_value (char* sysctl_key_value, char** sysctl_key, char** sysctl_value);
static int write_sysctl_to_file (char * sysctl_key, char* sysctl_value);

int configure_sysctls (char ** const sysctls, int size)
{
  char* key = NULL;
  char* value = NULL;

  for (int i = 0; i < size; ++i)
  {
    if (separate_sysctl_key_value (sysctls[i], &key, &value) < 0)
      return -1;

    if (write_sysctl_to_file (key, value) < 0)
      return -1;
  }

  return 0;
}

// key_value should be in the form `k=v`
static int separate_sysctl_key_value (char* key_value, char** key, char** value)
{
  // now find the `=` and convert it to a delimiter
  char * equals_token = strchr (key_value, '=');
  if (!equals_token)
  {
    nwarnf ("sysctl must be in the form of 'key=value'; '=' missing from %s", key_value);
    return -1;
  }

  // if the location of the equals sign is the beginning of the string
  // key is empty
  if (equals_token == key_value)
  {
    nwarnf ("sysctl must be in the form of 'key=value'; key is empty");
    return -1;
  }

  // we now have `k\0v`
  *equals_token = '\0';
  // key is now `k`
  *key = key_value;
  // equals_token is now `v`
  ++equals_token;
  // value is now `v`
  *value = equals_token;

  if (!strlen (*value))
  {
    nwarnf ("sysctl must be in the form of 'key=value'; value is empty");
    return -1;
  }

  return 0;
}

static int write_sysctl_to_file (char * sysctl_key, char* sysctl_value)
{
  if (!sysctl_key || !sysctl_value)
  {
    pwarn ("sysctl key or value not initialized");
    return -1;
  }

  // replace periods with / to create the sysctl path
  for (char* it = sysctl_key; *it; it++)
    if (*it == '.')
      *it = '/';

  _cleanup_close_ int dirfd = open ("/proc/sys", O_DIRECTORY | O_PATH | O_CLOEXEC);
  if (UNLIKELY (dirfd < 0))
  {
    pwarn ("failed to open /proc/sys");
    return -1;
  }

  _cleanup_close_ int fd = openat (dirfd, sysctl_key, O_WRONLY);
  if (UNLIKELY (fd < 0))
  {
    pwarnf ("failed to open /proc/sys/%s", sysctl_key);
    return -1;
  }

  int ret = TEMP_FAILURE_RETRY (write (fd, sysctl_value, strlen (sysctl_value)));
  if (UNLIKELY (ret < 0))
  {
    pwarnf ("failed to write to /proc/sys/%s", sysctl_key);
    return -1;
  }
  return 0;
}
