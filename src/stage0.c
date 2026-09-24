#include <stdio.h>
#include <stdlib.h>
#include <string.h>

typedef struct Block {
    char *content;
    struct Block *next;
} Block;

typedef struct Chunk {
    char *name;
    Block *first_block;
    Block *last_block;
    struct Chunk *next;
    int visiting;
} Chunk;

Chunk *chunks_head = NULL;

Chunk *find_chunk(const char *name) {
    Chunk *curr = chunks_head;
    while (curr) {
        if (strcmp(curr->name, name) == 0) {
            return curr;
        }
        curr = curr->next;
    }
    return NULL;
}

Chunk *get_or_create_chunk(const char *name) {
    Chunk *c = find_chunk(name);
    if (c) return c;
    c = malloc(sizeof(Chunk));
    if (!c) {
        fprintf(stderr, "Error: Memory allocation failed\n");
        exit(1);
    }
    c->name = strdup(name);
    c->first_block = NULL;
    c->last_block = NULL;
    c->next = chunks_head;
    c->visiting = 0;
    chunks_head = c;
    return c;
}

void append_block(Chunk *c, const char *content) {
    Block *b = malloc(sizeof(Block));
    if (!b) {
        fprintf(stderr, "Error: Memory allocation failed\n");
        exit(1);
    }
    b->content = strdup(content);
    b->next = NULL;
    if (c->last_block) {
        c->last_block->next = b;
        c->last_block = b;
    } else {
        c->first_block = b;
        c->last_block = b;
    }
}

void free_all() {
    Chunk *curr = chunks_head;
    while (curr) {
        Chunk *next_chunk = curr->next;
        free(curr->name);
        Block *b = curr->first_block;
        while (b) {
            Block *next_block = b->next;
            free(b->content);
            free(b);
            b = next_block;
        }
        free(curr);
        curr = next_chunk;
    }
}

char *read_entire_file(const char *filename, size_t *out_size) {
    FILE *f = fopen(filename, "rb");
    if (!f) {
        fprintf(stderr, "Error: Could not open file %s\n", filename);
        return NULL;
    }
    fseek(f, 0, SEEK_END);
    long size = ftell(f);
    if (size < 0) {
        fclose(f);
        return NULL;
    }
    fseek(f, 0, SEEK_SET);
    char *buf = malloc(size + 1);
    if (!buf) {
        fclose(f);
        return NULL;
    }
    size_t read_bytes = fread(buf, 1, size, f);
    buf[read_bytes] = '\0';
    fclose(f);
    if (out_size) *out_size = read_bytes;
    return buf;
}

void expand_chunk(const char *name);

void expand_block(const char *content) {
    size_t len = strlen(content);
    size_t i = 0;
    while (i < len) {
        if (i + 1 < len && content[i] == '@' && content[i + 1] == '<') {
            size_t ref_start = i + 2;
            size_t j = ref_start;
            int found_ref_end = 0;
            while (j < len) {
                if (j + 1 < len && content[j] == '@' && content[j + 1] == '>') {
                    found_ref_end = 1;
                    break;
                }
                j++;
            }
            if (!found_ref_end) {
                putchar(content[i]);
                i++;
            } else {
                size_t ref_len = j - ref_start;
                char *ref_name = malloc(ref_len + 1);
                if (!ref_name) {
                    fprintf(stderr, "Error: Memory allocation failed\n");
                    exit(1);
                }
                strncpy(ref_name, content + ref_start, ref_len);
                ref_name[ref_len] = '\0';
                
                expand_chunk(ref_name);
                
                free(ref_name);
                i = j + 2;
            }
        } else {
            putchar(content[i]);
            i++;
        }
    }
}

void expand_chunk(const char *name) {
    Chunk *c = find_chunk(name);
    if (!c) {
        return;
    }
    if (c->visiting) {
        fprintf(stderr, "Error: Circular dependency detected for chunk '%s'\n", name);
        exit(1);
    }
    c->visiting = 1;
    Block *b = c->first_block;
    while (b) {
        expand_block(b->content);
        b = b->next;
    }
    c->visiting = 0;
}

int main(int argc, char **argv) {
    if (argc != 4 || strcmp(argv[1], "-p") != 0) {
        fprintf(stderr, "Usage: %s -p <chunk_name> <archive_file>\n", argv[0]);
        return 1;
    }
    const char *chunk_to_print = argv[2];
    const char *archive_file = argv[3];

    size_t size = 0;
    char *buf = read_entire_file(archive_file, &size);
    if (!buf) {
        return 1;
    }

    size_t pos = 0;
    while (pos < size) {
        if (pos + 1 < size && buf[pos] == '@' && buf[pos + 1] == '<') {
            pos += 2;
            size_t name_start = pos;
            int found_eq = 0;
            while (pos < size) {
                if (pos + 1 < size && buf[pos] == '@' && buf[pos + 1] == '=') {
                    found_eq = 1;
                    break;
                }
                pos++;
            }
            if (!found_eq) {
                fprintf(stderr, "Error: Expected '@=' after chunk name starting at position %zu\n", name_start);
                free(buf);
                free_all();
                return 1;
            }
            size_t name_len = pos - name_start;
            char *chunk_name = malloc(name_len + 1);
            if (!chunk_name) {
                fprintf(stderr, "Error: Memory allocation failed\n");
                exit(1);
            }
            strncpy(chunk_name, buf + name_start, name_len);
            chunk_name[name_len] = '\0';
            
            pos += 2;
            
            size_t content_start = pos;
            int found_end = 0;
            while (pos < size) {
                if (buf[pos] == '@') {
                    if (pos + 1 < size) {
                        char next_c = buf[pos + 1];
                        if (next_c == ' ' || next_c == '\t' || next_c == '\n' || next_c == '\r') {
                            found_end = 1;
                            break;
                        }
                    } else {
                        found_end = 1;
                        break;
                    }
                }
                pos++;
            }
            if (!found_end) {
                fprintf(stderr, "Error: Expected '@' followed by space/tab/newline to end block content starting at position %zu\n", content_start);
                free(chunk_name);
                free(buf);
                free_all();
                return 1;
            }
            size_t block_start = content_start;
            size_t block_end = pos;
            
            if (block_start < block_end && buf[block_start] == '\r') {
                block_start++;
            }
            if (block_start < block_end && buf[block_start] == '\n') {
                block_start++;
            }
            
            if (block_end > block_start && buf[block_end - 1] == '\n') {
                block_end--;
                if (block_end > block_start && buf[block_end - 1] == '\r') {
                    block_end--;
                }
            } else if (block_end > block_start && buf[block_end - 1] == '\r') {
                block_end--;
            }
            
            size_t trimmed_len = block_end - block_start;
            char *block_content = malloc(trimmed_len + 1);
            if (!block_content) {
                fprintf(stderr, "Error: Memory allocation failed\n");
                exit(1);
            }
            strncpy(block_content, buf + block_start, trimmed_len);
            block_content[trimmed_len] = '\0';
            
            Chunk *c = get_or_create_chunk(chunk_name);
            append_block(c, block_content);
            
            free(chunk_name);
            free(block_content);
            
            pos++;
            if (pos < size) {
                char next_c = buf[pos];
                if (next_c == ' ' || next_c == '\t' || next_c == '\n' || next_c == '\r') {
                    if (next_c == '\r' && pos + 1 < size && buf[pos + 1] == '\n') {
                        pos += 2;
                    } else {
                        pos++;
                    }
                }
            }
        } else {
            pos++;
        }
    }

    expand_chunk(chunk_to_print);

    free(buf);
    free_all();
    return 0;
}
