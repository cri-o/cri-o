SUBDIRS = crun/libocispec
DIST_SUBDIRS = crun/libocispec
ACLOCAL_AMFLAGS = -I m4

bin_PROGRAMS = pinns

pinns_SOURCES = src/pinns.c

LIBOCISPEC_SOURCES = -I $(abs_top_builddir)/crun/libocispec/src -I $(abs_top_srcdir)/crun/libocispec/src
LIBCRUN_SOURCES = -I $(abs_top_builddir)/crun/src/libcrun -I $(abs_top_srcdir)/crun/src/libcrun
pinns_LDADD = crun/libocispec/libocispec.la crun/libcrun.la
pinns_CFLAGS = $(LIBOCISPEC_SOURCES) $(LIBCRUN_SOURCES) -static -std=c99 -Os -Wall -Werror -Wextra
