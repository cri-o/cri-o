#pragma once
#if !defined(SYSCTL_H)
#define SYSCTL_H

#include <glib.h>

int configure_sysctls (const GPtrArray *sysctls);

#endif // SYSCTL_H
