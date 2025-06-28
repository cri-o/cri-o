#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <sys/mman.h>
#include <unistd.h>

#define HUGE_PAGE_SIZE (2 * 1024 * 1024) // 2MB

int main(int argc, char *argv[]) {
    void *addr = mmap(NULL, HUGE_PAGE_SIZE, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANONYMOUS | MAP_HUGETLB, NULL, 0);
    if (addr == MAP_FAILED) {
        perror("Failed to map a huge page");
        exit(EXIT_FAILURE);
    }

    // Allow some time to collect usage metrics
    sleep(10);

    if (munmap(addr, HUGE_PAGE_SIZE) == MAP_FAILED) {
        perror("Failed to unmap the huge page");
        exit(EXIT_FAILURE);
    }
}
