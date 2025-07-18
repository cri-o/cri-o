#define _GNU_SOURCE
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <sys/mman.h>
#include <unistd.h>

#define HUGE_PAGE_SIZE (2 * 1024 * 1024) // 2MB

static void *addr = NULL;

static void cleanup_huge_page() {
    if (munmap(addr, HUGE_PAGE_SIZE) == MAP_FAILED) {
        perror("Failed to unmap the huge page");
        exit(EXIT_FAILURE);
    }
    exit(EXIT_SUCCESS);
}

int main(int argc, char *argv[]) {
    // Register SIGTERM handler
    signal(SIGTERM, cleanup_huge_page);

    // Trigger huge page usage
    addr = mmap(NULL, HUGE_PAGE_SIZE, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANONYMOUS | MAP_HUGETLB, NULL, 0);
    if (addr == MAP_FAILED) {
        perror("Failed to map a huge page");
        exit(EXIT_FAILURE);
    }

    // Hold on to the huge page until SIGTERM
    sleep(100);
    cleanup_huge_page();
}
