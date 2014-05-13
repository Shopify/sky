package query

/*
#cgo LDFLAGS: -L/usr/local/lib -lluajit-5.1
#cgo CFLAGS: -I/usr/local/include

#include <stdio.h>
#include <stdlib.h>
#include <stdbool.h>
#include <inttypes.h>
#include <string.h>
#include <luajit-2.0/lua.h>
#include <luajit-2.0/lualib.h>
#include <luajit-2.0/lauxlib.h>

int mp_pack(lua_State *L);
int mp_unpack(lua_State *L);


//==============================================================================
//
// BOLT
//
//==============================================================================

//------------------------------------------------------------------------------
// Constants
//------------------------------------------------------------------------------

// This represents the maximum number of levels that a cursor can traverse.
#define MAX_DEPTH   64

// These flags mark the type of page and are set in the page.flags.
#define PAGE_BRANCH   0x01
#define PAGE_LEAF     0x02
#define PAGE_META     0x04
#define PAGE_FREELIST 0x10


//------------------------------------------------------------------------------
// Typedefs
//------------------------------------------------------------------------------

// These types MUST have the same layout as their corresponding Go types

typedef int64_t pgid;

// Page represents a header struct of a block in the mmap.
typedef struct page {
    pgid     id;
    uint16_t flags;
    uint16_t count;
    uint32_t overflow;
} page;

typedef struct bucket {
    pgid     root;
    uint64_t sequence;
} bucket;

// The branch element represents an a item in a branch page
// that points to a child page.
typedef struct branch_element {
    uint32_t pos;
    uint32_t ksize;
    pgid     pgid;
} branch_element;

// The leaf element represents an a item in a leaf page
// that points to a key/value pair.
typedef struct leaf_element {
    uint32_t flags;
    uint32_t pos;
    uint32_t ksize;
    uint32_t vsize;
} leaf_element;

// elem_ref represents a pointer to an element inside of a page.
// It is used by the cursor stack to track the position at each level.
typedef struct elem_ref {
    page     *page;
    uint16_t index;
} elem_ref;

// bolt_val represents a pointer to a fixed-length series of bytes.
// It is used to represent keys and values returned by the cursor.
typedef struct bolt_val {
    uint32_t size;
    void     *data;
} bolt_val;

// bolt_cursor represents a cursor attached to a bucket.
typedef struct bolt_cursor {
    void     *data;
    pgid     root;
    size_t   pgsz;
    int      top;
    elem_ref stack[MAX_DEPTH];
} bolt_cursor;


//------------------------------------------------------------------------------
// Forward Declarations
//------------------------------------------------------------------------------

elem_ref *cursor_push(bolt_cursor *c, pgid id);

elem_ref *cursor_current(bolt_cursor *c);

elem_ref *cursor_pop(bolt_cursor *c);

void cursor_key_value(bolt_cursor *c, bolt_val *key, bolt_val *value, uint32_t *flags);

void cursor_search(bolt_cursor *c, bolt_val key, pgid id);

void cursor_search_branch(bolt_cursor *c, bolt_val key);

void cursor_search_leaf(bolt_cursor *c, bolt_val key);

//------------------------------------------------------------------------------
// Public Functions
//------------------------------------------------------------------------------

// Initializes a cursor.
void bolt_cursor_init(bolt_cursor *c, void *data, size_t pgsz, pgid root) {
    c->data = data;
    c->root = root;
    c->pgsz = pgsz;
    c->top = -1;
}

// Positions the cursor to the first leaf element and returns the key/value pair.
void bolt_cursor_first(bolt_cursor *c, bolt_val *key, bolt_val *value, uint32_t *flags) {
    // reset stack to initial state
    elem_ref *ref = cursor_push(c, c->root);

    // Find first leaf and return key/value.
    cursor_key_value(c, key, value, flags);
}

// Positions the cursor to the next leaf element and returns the key/value pair.
void bolt_cursor_next(bolt_cursor *c, bolt_val *key, bolt_val *value, uint32_t *flags) {
    int i;
    elem_ref *ref;

    // Attempt to move over one element until we're successful.
    // Move up the stack as we hit the end of each page in our stack.
    for (ref = cursor_current(c); ref != NULL; ref = cursor_current(c)) {
        ref->index++;
        if (ref->index < ref->page->count) break;
        cursor_pop(c);
    };

    // Find first leaf and return key/value.
    cursor_key_value(c, key, value, flags);
}

// Positions the cursor first leaf element starting from a given key.
// If there is a matching key then the cursor will be place on that key.
// If there not a match then the cursor will be placed on the next key, if available.
void bolt_cursor_seek(bolt_cursor *c, bolt_val seek, bolt_val *key, bolt_val *value, uint32_t *flags) {
    // Start from root page/node and traverse to correct page.
    cursor_push(c, c->root);
    if (seek.size > 0) cursor_search(c, seek, c->root);

    // Find first leaf and return key/value.
    cursor_key_value(c, key, value, flags);
}


//------------------------------------------------------------------------------
// Private Functions
//------------------------------------------------------------------------------

// Push ref to the first element of the page onto the cursor stack
// If the page is the root page reset the stack to initial state
elem_ref *cursor_push(bolt_cursor *c, pgid id) {
    elem_ref *ref;
    if (id == c->root)
        c->top = 0;
    else
        c->top++;
    ref = &(c->stack[c->top]);
    ref->page = (page *)(c->data + (c->pgsz * id));
    ref->index = 0;
    return ref;
}

// Return current element ref from the cursor stack
// If stack is empty return null
elem_ref *cursor_current(bolt_cursor *c) {
    if (c->top < 0) return NULL;
    return &c->stack[c->top];
}

// Pop current element ref off the cursor stack
// If stack is empty return null
elem_ref *cursor_pop(bolt_cursor *c) {
    elem_ref *ref = cursor_current(c);
    if (ref != NULL) c->top--;
    return ref;
}

// Returns the branch element at a given index on a given page.
branch_element *page_branch_element(page *p, uint16_t index) {
    branch_element *elements = (branch_element*)((void*)(p) + sizeof(page));
    return &elements[index];
}

// Returns the leaf element at a given index on a given page.
leaf_element *page_leaf_element(page *p, uint16_t index) {
    leaf_element *elements = (leaf_element*)((void*)(p) + sizeof(page));
    return &elements[index];
}

// Returns the key/value pair for the current position of the cursor.
void cursor_key_value(bolt_cursor *c, bolt_val *key, bolt_val *value, uint32_t *flags) {
    elem_ref *ref = cursor_current(c);

    // If stack or current page is empty return null.
    if (ref == NULL || ref->page->count == 0) {
        key->size = value->size = 0;
        key->data = value->data = NULL;
        *flags = 0;
        return;
    };

    // Descend to the current leaf page if we're on branch page.
    while (ref->page->flags & PAGE_BRANCH) {
        branch_element *elem = page_branch_element(ref->page,ref->index);
        ref = cursor_push(c, elem->pgid);
    };

    leaf_element *elem = page_leaf_element(ref->page,ref->index);

    // Assign key pointer.
    key->size = elem->ksize;
    key->data = ((void*)elem) + elem->pos;

    // Assign value pointer.
    value->size = elem->vsize;
    value->data = key->data + key->size;

    // Return the element flags.
    *flags = elem->flags;
}

// Recursively performs a binary search against a given page/node until it finds a given key.
void cursor_search(bolt_cursor *c, bolt_val key, pgid id) {
    // Push page onto the cursor stack.
    elem_ref *ref = cursor_push(c, id);

    // If we're on a leaf page/node then find the specific node.
    if (ref->page->flags & PAGE_LEAF) {
        cursor_search_leaf(c, key);
        return;
    }

    // Otherwise search the branch page.
    cursor_search_branch(c, key);
}

// Recursively search over a leaf page for a key.
void cursor_search_leaf(bolt_cursor *c, bolt_val key) {
    elem_ref *ref = cursor_current(c);
    int i;

    // HACK: Simply loop over elements to find the right one. Replace with a binary search.
    leaf_element *elems = (leaf_element*)((void*)(ref->page) + sizeof(page));
    for (i=0; i<ref->page->count; i++) {
        leaf_element *elem = &elems[i];
        int rc = memcmp(key.data, ((void*)elem) + elem->pos, (elem->ksize < key.size ? elem->ksize : key.size));

        if ((rc == 0 && key.size <= elem->ksize) || rc < 0) {
            ref->index = i;
            return;
        }
    }

    // If nothing was greater than the key then pop the current page off the stack.
    cursor_pop(c);
}

// Recursively search over a branch page for a key.
void cursor_search_branch(bolt_cursor *c, bolt_val key) {
    elem_ref *ref = cursor_current(c);
    int i;

    // HACK: Simply loop over elements to find the right one. Replace with a binary search.
    branch_element *elems = (branch_element*)((void*)(ref->page) + sizeof(page));
    for (i=0; i<ref->page->count; i++) {
        branch_element *elem = &elems[i];
        int rc = memcmp(key.data, ((void*)elem) + elem->pos, (elem->ksize < key.size ? elem->ksize : key.size));

        if (rc == 0 && key.size == elem->ksize) {
            // Exact match, done.
            ref->index = i;
            return;
        } else if ((rc == 0 && key.size < elem->ksize) || rc < 0) {
            // If key is less than anything in this subtree we are done.
            // This should really only happen for key that's less than anything in the tree.
            if (i == 0) return;
            // Otherwise search the previous subtree.
            cursor_search(c, key, elems[i-1].pgid);
            // Didn't find anything greater than key?
            if (cursor_current(c) == ref)
                ref->index = i;
            else
                ref->index = i-1;
            return;
        }
    }

    // If nothing was greater than the key then search the last child.
    cursor_search(c, key, elems[ref->page->count-1].pgid);
    // If still didn't find anything greater than key, then pop the page off the stack.
    if (cursor_current(c) == ref)
        cursor_pop(c);
    else
        ref->index = ref->page->count-1;
}






//==============================================================================
//
// Constants
//
//==============================================================================

// The number of microseconds per second.
#define USEC_PER_SEC        1000000

// A bit-mask to extract the microseconds from a Sky timestamp.
#define USEC_MASK           0xFFFFF

// The number of bits that seconds are shifted over in a timestamp.
#define SECONDS_BIT_OFFSET  20


//==============================================================================
//
// Macros
//
//==============================================================================

#define memdump(PTR, LENGTH) do {\
    char *address = (char*)PTR;\
    int length = LENGTH;\
    int i = 0;\
    char *line = (char*)address;\
    unsigned char ch;\
    fprintf(stderr, "%"PRIX64" | ", (int64_t)address);\
    while (length-- > 0) {\
        fprintf(stderr, "%02X ", (unsigned char)*address++);\
        if (!(++i % 16) || (length == 0 && i % 16)) {\
            if (length == 0) { while (i++ % 16) { fprintf(stderr, "__ "); } }\
            fprintf(stderr, "| ");\
            while (line < address) {\
                ch = *line++;\
                fprintf(stderr, "%c", (ch < 33 || ch == 255) ? 0x2E : ch);\
            }\
            if (length > 0) { fprintf(stderr, "\n%09X | ", (int)address); }\
        }\
    }\
    fprintf(stderr, "\n\n");\
} while(0)

// Removed from macro below.
// memdump(cursor->startptr, (cursor->endptr - cursor->startptr));

#define badcursordata(MSG, PTR) do {\
    fprintf(stderr, "Cursor pointing at invalid raw event data [" MSG "]: %p\n", PTR); \
    cursor->next_event->timestamp = 0; \
    return false; \
} while(0)

#define debug(M, ...) fprintf(stderr, "DEBUG %s:%d: " M "\n", __FILE__, __LINE__, ##__VA_ARGS__)


//==============================================================================
//
// Typedefs
//
//==============================================================================

typedef struct {
  int32_t length;
  char *data;
} sky_string;

typedef struct sky_cursor sky_cursor;

typedef int (*sky_cursor_next_object_func)(void *cursor);

typedef void (*sky_property_descriptor_set_func)(void *target, void *value, size_t *sz);
typedef void (*sky_property_descriptor_clear_func)(void *target);

typedef struct { uint16_t ts_offset; uint16_t timestamp_offset;} sky_timestamp_descriptor;

typedef struct {
    int64_t property_id;
    uint16_t offset;
    sky_property_descriptor_set_func set_func;
    sky_property_descriptor_clear_func clear_func;
} sky_property_descriptor;

typedef struct {
  bool eos;
  bool eof;
  uint32_t timestamp;
  int64_t ts;
} sky_event;

struct sky_cursor {
    sky_event *event;
    sky_event *next_event;

    uint32_t max_timestamp;
    uint32_t session_idle_in_sec;

    bool eos_wait;
    uint32_t event_sz;
    uint32_t action_event_sz;
    uint32_t variable_event_sz;

    sky_timestamp_descriptor timestamp_descriptor;
    sky_property_descriptor *property_descriptors;
    sky_property_descriptor *property_zero_descriptor;
    uint32_t property_count;

    int32_t min_property_id;
    int32_t max_property_id;

    void *key_prefix;
    uint32_t key_prefix_sz;
    bolt_cursor object_cursor;
    bolt_cursor event_cursor;

    bolt_val min_event_key;
    bolt_val max_event_key;
};

//==============================================================================
//
// Forward Declarations
//
//==============================================================================

//--------------------------------------
// Setters
//--------------------------------------

void sky_set_noop(void *target, void *value, size_t *sz);

void sky_set_string(void *target, void *value, size_t *sz);

void sky_set_int(void *target, void *value, size_t *sz);

void sky_set_double(void *target, void *value, size_t *sz);

void sky_set_boolean(void *target, void *value, size_t *sz);

//--------------------------------------
// Clear Functions
//--------------------------------------

void sky_clear_string(void *target);

void sky_clear_int(void *target);

void sky_clear_double(void *target);

void sky_clear_boolean(void *target);

//--------------------------------------
// Object Iteration
//--------------------------------------

bool sky_cursor_first_object(sky_cursor *cursor);

bool sky_cursor_next_object(sky_cursor *cursor);

bool sky_cursor_read(sky_cursor *cursor, sky_event *event, void *ptr);

bool sky_cursor_next_event(sky_cursor *cursor);

bool sky_cursor_eof(sky_cursor *cursor);

bool sky_cursor_eos(sky_cursor *cursor);

void sky_cursor_update_eos(sky_cursor *cursor);

//--------------------------------------
// Timestamps
//--------------------------------------

int64_t sky_timestamp_shift(int64_t value);

int64_t sky_timestamp_unshift(int64_t value);

int64_t sky_timestamp_to_seconds(int64_t value);

//--------------------------------------
// Minipack
//--------------------------------------

size_t minipack_sizeof_elem_and_data(void *ptr);

bool minipack_is_raw(void *ptr);

int64_t minipack_unpack_int(void *ptr, size_t *sz);

double minipack_unpack_double(void *ptr, size_t *sz);

bool minipack_unpack_bool(void *ptr, size_t *sz);

uint32_t minipack_unpack_raw(void *ptr, size_t *sz);

uint32_t minipack_unpack_map(void *ptr, size_t *sz);

void minipack_unpack_nil(void *ptr, size_t *sz);


//==============================================================================
//
// Byte Order
//
//==============================================================================

#include <sys/types.h>

#ifndef BYTE_ORDER
#if defined(linux) || defined(__linux__)
# include <endian.h>
#else
# include <machine/endian.h>
#endif
#endif

#if !defined(BYTE_ORDER) && !defined(__BYTE_ORDER)
#error "Undefined byte order"
#endif

uint64_t bswap64(uint64_t value);

#if (BYTE_ORDER == LITTLE_ENDIAN) || (__BYTE_ORDER == __LITTLE_ENDIAN)
#define htonll(x) bswap64(x)
#define ntohll(x) bswap64(x)
#else
#define htonll(x) x
#define ntohll(x) x
#endif


//==============================================================================
//
// Functions
//
//==============================================================================

//--------------------------------------
// Lifecycle
//--------------------------------------

// Creates a reference to a cursor.
sky_cursor *sky_cursor_new(int32_t min_property_id,
                           int32_t max_property_id)
{
    sky_cursor *cursor = calloc(1, sizeof(sky_cursor));
    if(cursor == NULL) debug("[malloc] Unable to allocate cursor.");

    // Add one property to account for the zero descriptor.
    int32_t property_count = (max_property_id - min_property_id) + 1;

    // Allocate memory for the descriptors.
    cursor->property_descriptors = calloc(property_count, sizeof(sky_property_descriptor));
    if(cursor->property_descriptors == NULL) debug("[malloc] Unable to allocate property descriptors.");
    cursor->property_count = property_count;
    cursor->property_zero_descriptor = NULL;

    cursor->min_property_id = min_property_id;
    cursor->max_property_id = max_property_id;

    // Initialize all property descriptors to noop.
    int32_t i;
    for(i=0; i<property_count; i++) {
        int64_t property_id = min_property_id + (int64_t)i;
        cursor->property_descriptors[i].property_id = property_id;
        cursor->property_descriptors[i].set_func = sky_set_noop;

        // Save a pointer to the descriptor that points to property zero.
        if(property_id == 0) {
            cursor->property_zero_descriptor = &cursor->property_descriptors[i];
        }
    }

    return cursor;
}

// Removes a cursor reference from memory.
void sky_cursor_free(sky_cursor *cursor)
{
    if(cursor) {
        if(cursor->property_descriptors != NULL) free(cursor->property_descriptors);
        cursor->property_zero_descriptor = NULL;
        cursor->property_count = 0;

        if(cursor->event != NULL) free(cursor->event);
        cursor->event = NULL;
        if(cursor->next_event != NULL) free(cursor->next_event);
        cursor->next_event = NULL;
        if(cursor->key_prefix != NULL) free(cursor->key_prefix);
        cursor->key_prefix = NULL;

        if(cursor->min_event_key.data != NULL) free(cursor->min_event_key.data);
        cursor->min_event_key.data = NULL;
        cursor->min_event_key.size = 0;

        if(cursor->max_event_key.data != NULL) free(cursor->max_event_key.data);
        cursor->max_event_key.data = NULL;
        cursor->max_event_key.size = 0;

        free(cursor);
    }
}


//--------------------------------------
// Data Management
//--------------------------------------

void sky_cursor_set_value(sky_cursor *cursor, void *target,
                          int64_t property_id, void *ptr, size_t *sz)
{
    if(property_id >= cursor->min_property_id && property_id <= cursor->max_property_id) {
        sky_property_descriptor *property_descriptor = &cursor->property_zero_descriptor[property_id];
        property_descriptor->set_func(target + property_descriptor->offset, ptr, sz);
    } else {
        sky_set_noop(NULL, ptr, sz);
    }
}


//--------------------------------------
// Descriptor Management
//--------------------------------------

void sky_cursor_set_event_sz(sky_cursor *cursor, uint32_t sz) {
    cursor->event_sz = sz;

    if(cursor->event != NULL) free(cursor->event);
    cursor->event = calloc(1, sz);
    if(cursor->event == NULL) debug("[malloc] Unable to allocate cursor event.");

    if(cursor->next_event != NULL) free(cursor->next_event);
    cursor->next_event = calloc(1, sz);
    if(cursor->next_event == NULL) debug("[malloc] Unable to allocate cursor next event.");
}

// Sets the data type and offset for a given property id.
void sky_cursor_set_property(sky_cursor *cursor, int64_t property_id,
                             uint32_t offset, uint32_t sz, const char *data_type)
{
    sky_property_descriptor *property_descriptor = &cursor->property_zero_descriptor[property_id];

    // Set the offset and set_func function on the descriptor.
    if(property_id != 0) {
        property_descriptor->offset = offset;
        if(strlen(data_type) == 0) {
            property_descriptor->set_func = sky_set_noop;
            property_descriptor->clear_func = NULL;
        }
        else if(strcmp(data_type, "string") == 0) {
            property_descriptor->set_func = sky_set_string;
            property_descriptor->clear_func = sky_clear_string;
        }
        else if(strcmp(data_type, "factor") == 0 || strcmp(data_type, "integer") == 0) {
            property_descriptor->set_func = sky_set_int;
            property_descriptor->clear_func = sky_clear_int;
        }
        else if(strcmp(data_type, "float") == 0) {
            property_descriptor->set_func = sky_set_double;
            property_descriptor->clear_func = sky_clear_double;
        }
        else if(strcmp(data_type, "boolean") == 0) {
            property_descriptor->set_func = sky_set_boolean;
            property_descriptor->clear_func = sky_clear_boolean;
        }
        else {
            property_descriptor->set_func = sky_set_boolean;
            property_descriptor->clear_func = sky_clear_boolean;
        }
    }

    // Resize the action data area. This area occurs after the
    // fixed fields in the struct.
    int32_t new_action_event_sz = (offset + sz) - sizeof(sky_event);
    if(property_id < 0 && new_action_event_sz > cursor->action_event_sz) {
        cursor->action_event_sz = (uint32_t)new_action_event_sz;
    }

    // Resize the variable data area. This area occurs after the
    // action data fields in the struct.
    int32_t new_variable_event_sz = (offset + sz) - sizeof(sky_event);
    if(property_id == 0 && new_variable_event_sz > 0 && new_variable_event_sz > cursor->variable_event_sz) {
        cursor->variable_event_sz = (uint32_t)new_variable_event_sz;
    }
}


//--------------------------------------
// Object Iteration
//--------------------------------------

// Sets up object after cursor has already been positioned.
bool sky_cursor_iter_object(sky_cursor *cursor, bolt_val *key, bolt_val *data)
{

    if(cursor->key_prefix != NULL && (key->size < cursor->key_prefix_sz || memcmp(cursor->key_prefix, key->data, cursor->key_prefix_sz) != 0)) {
        return false;
    }
    // fprintf(stderr, "\nOBJ (%.*s) [%d]\n", (int)key->mv_size, (char*)key->mv_data, (int)key->mv_size);

    // Clear the data object if set.
    cursor->session_idle_in_sec = 0;
    cursor->eos_wait = false;
    cursor->next_event->eof = false;
    memset(cursor->event, 0, cursor->event_sz);

    // Extract the bucket from the object cursor and init event cursor.
    bucket *b = (bucket*)data->data;
    bolt_cursor_init(&cursor->event_cursor, cursor->object_cursor.data, cursor->object_cursor.pgsz, b->root);

    // Read the first event into the cursor buffer.
    uint32_t flags;
    bolt_val event_key, event_data;
    bolt_cursor_seek(&cursor->event_cursor, cursor->min_event_key, &event_key, &event_data, &flags);

    // Make sure the first event is not past the timestamp range.
    if(cursor->max_event_key.size > 0 && memcmp(event_key.data, cursor->max_event_key.data, cursor->max_event_key.size) > 0) {
        return false;
    }

    if(!sky_cursor_read(cursor, cursor->next_event, event_data.data)) {
        return false;
    }

    // Move "next" event to current event and put the next event in buffer.
    return sky_cursor_next_event(cursor);
}

// Moves the cursor to point to the first object. If a prefix is set then
// move to the first object that with the given prefix.
bool sky_cursor_first_object(sky_cursor *cursor)
{
    uint32_t flags;
    bolt_val key, data, seek;

    if(cursor->key_prefix == NULL) {
        bolt_cursor_first(&cursor->object_cursor, &key, &data, &flags);
        if (key.size == 0) {
            return false;
        }

    } else {
        seek.data = cursor->key_prefix;
        seek.size = cursor->key_prefix_sz;
        bolt_cursor_seek(&cursor->object_cursor, seek, &key, &data, &flags);

        if (key.size == 0) {
            return false;
        }
    }

    return sky_cursor_iter_object(cursor, &key, &data);
}

// Moves the cursor to point to the next object.
bool sky_cursor_next_object(sky_cursor *cursor)
{
    // Move to next object.
    uint32_t flags;
    bolt_val key, data;
    bolt_cursor_next(&cursor->object_cursor, &key, &data, &flags);
    if(key.size == 0) {
        return false;
    }

    return sky_cursor_iter_object(cursor, &key, &data);
}

// Moves the cursor to point to the next event.
// Returns true if the cursor moved forward, otherwise false.
bool sky_cursor_next_event(sky_cursor *cursor)
{
    // Don't allow cursor to move if we're EOF or marked as EOS wait.
    if(cursor->event->eof || (cursor->event->eos && cursor->eos_wait)) {
        return false;
    }
    cursor->eos_wait = true;

    // Copy variable state from current event to next event.
    if(cursor->variable_event_sz > 0) {
        uint32_t variable_event_offset = sizeof(sky_event) + cursor->action_event_sz;
        memcpy(((void*)cursor->next_event) + variable_event_offset, ((void*)cursor->event) + variable_event_offset, cursor->variable_event_sz - cursor->action_event_sz);
    }

    // Copy the next event to the current event.
    memcpy(cursor->event, cursor->next_event, cursor->event_sz);

    // Read the next event.
    if(!cursor->next_event->eof) {
        uint32_t flags;
        bolt_val key, data;
        bolt_cursor_next(&cursor->event_cursor, &key, &data, &flags);

        // Clear next event if there isn't one or if the next timestamp is
        // beyond our max event key.
        if(key.size == 0 || (cursor->max_event_key.size > 0 && memcmp(key.data, cursor->max_event_key.data, cursor->max_event_key.size) > 0)) {
            memset(cursor->next_event, 0, cursor->event_sz);
            cursor->next_event->eof = true;
        } else {
            cursor->next_event->eof = false;
            if(!sky_cursor_read(cursor, cursor->next_event, data.data)) {
                return true;
            }
        }
    }

    // Update eos.
    sky_cursor_update_eos(cursor);

    return true;
}

// Reads the data at a given pointer into a data object.
bool sky_cursor_read(sky_cursor *cursor, sky_event *event, void *ptr)
{
    // Set timestamp.
    event->ts = htonll(*((int64_t*)ptr));
    event->timestamp = sky_timestamp_to_seconds(event->ts);
    ptr += 8;

    // Clear old action data.
    if(cursor->action_event_sz > 0) {
        memset(&event[1], 0, cursor->action_event_sz);
    }

    // Read msgpack map!
    size_t sz;
    uint32_t count = minipack_unpack_map(ptr, &sz);
    if(sz == 0) {
      minipack_unpack_nil(ptr, &sz);
      if(sz == 0) {
        badcursordata("datamap", ptr);
      }
    }
    ptr += sz;

    // Loop over key/value pairs.
    uint32_t i;
    for(i=0; i<count; i++) {
        // Read property id (key).
        int64_t property_id = minipack_unpack_int(ptr, &sz);
        if(sz == 0) badcursordata("key", ptr);
        ptr += sz;

        // Read property value and set it on the data object.
        sky_cursor_set_value(cursor, event, property_id, ptr, &sz);
        if(sz == 0) {
          debug("[invalid read, skipping]");
          sz = minipack_sizeof_elem_and_data(ptr);
        }
        ptr += sz;
    }

    return true;
}

bool sky_lua_cursor_next_event(sky_cursor *cursor)
{
    return sky_cursor_next_event(cursor);
}

bool sky_cursor_eof(sky_cursor *cursor)
{
    return cursor->event->eof;
}

// End-of-session (EOS) is defined by idle time between the current event and the next event.
bool sky_cursor_eos(sky_cursor *cursor)
{
    return cursor->event->eos;
}

// Updates the end-of-session flag on the current event.
void sky_cursor_update_eos(sky_cursor *cursor)
{
    if(cursor->next_event->eof) {
        cursor->event->eos = true;
    } else if(cursor->session_idle_in_sec == 0) {
        cursor->event->eos = false;
    } else {
        cursor->event->eos = (cursor->next_event->timestamp - cursor->event->timestamp >= cursor->session_idle_in_sec);
    }
}

void sky_cursor_set_session_idle(sky_cursor *cursor, uint32_t seconds)
{
    cursor->session_idle_in_sec = seconds;
    sky_cursor_update_eos(cursor);
}

void sky_cursor_next_session(sky_cursor *cursor)
{
    cursor->eos_wait = false;
}

bool sky_lua_cursor_next_session(sky_cursor *cursor)
{
    sky_cursor_next_session(cursor);
    return !cursor->next_event->eof;
}



//--------------------------------------
// Setters
//--------------------------------------

void sky_set_noop(void *target, void *value, size_t *sz)
{
    ((void)(target));
    *sz = minipack_sizeof_elem_and_data(value);
}

void sky_set_string(void *target, void *value, size_t *sz)
{
    size_t _sz;
    sky_string *string = (sky_string*)target;
    string->length = minipack_unpack_raw(value, &_sz);
    string->data = (_sz > 0 ? value + _sz : NULL);
    *sz = _sz + string->length;
}

void sky_set_int(void *target, void *value, size_t *sz)
{
    *((int32_t*)target) = (int32_t)minipack_unpack_int(value, sz);
    if(*sz == 0) {
      minipack_unpack_nil(value, sz);
      if(*sz != 0) {
        *((int32_t*)target) = 0;
      }
    }
}

void sky_set_double(void *target, void *value, size_t *sz)
{
    *((double*)target) = minipack_unpack_double(value, sz);
    if(*sz == 0) {
      minipack_unpack_nil(value, sz);
      if(*sz != 0) {
        *((double*)target) = 0;
      }
    }
}

void sky_set_boolean(void *target, void *value, size_t *sz)
{
    *((bool*)target) = minipack_unpack_bool(value, sz);
    if(*sz == 0) {
      minipack_unpack_nil(value, sz);
      if(*sz != 0) {
        *((bool*)target) = false;
      }
    }
}


//--------------------------------------
// Clear Functions
//--------------------------------------

void sky_clear_string(void *target)
{
    sky_string *string = (sky_string*)target;
    string->length = 0;
    string->data = NULL;
}

void sky_clear_int(void *target)
{
    *((int32_t*)target) = 0;
}

void sky_clear_double(void *target)
{
    *((double*)target) = 0;
}

void sky_clear_boolean(void *target)
{
    *((bool*)target) = false;
}

//--------------------------------------
// Timestamps
//--------------------------------------

// Converts a timestamp from the number of microseconds since the epoch to
// a bit-shifted Sky timestamp.
//
// value - Microseconds since the unix epoch.
//
// Returns a bit-shifted Sky timestamp.
int64_t sky_timestamp_shift(int64_t value)
{
    int64_t usec = value % USEC_PER_SEC;
    int64_t sec  = (value / USEC_PER_SEC);

    return (sec << SECONDS_BIT_OFFSET) + usec;
}

// Converts a bit-shifted Sky timestamp to the number of microseconds since
// the Unix epoch.
//
// value - Sky timestamp.
//
// Returns the number of microseconds since the Unix epoch.
int64_t sky_timestamp_unshift(int64_t value)
{
    int64_t usec = value & USEC_MASK;
    int64_t sec  = value >> SECONDS_BIT_OFFSET;

    return (sec * USEC_PER_SEC) + usec;
}

// Converts a bit-shifted Sky timestamp to seconds since the epoch.
//
// value - Sky timestamp.
//
// Returns the number of seconds since the Unix epoch.
int64_t sky_timestamp_to_seconds(int64_t value)
{
    return (value >> SECONDS_BIT_OFFSET);
}

*/
import "C"

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"text/template"
	"unsafe"

	"github.com/boltdb/bolt"
	"github.com/skydb/sky/db"
	"github.com/ugorji/go/codec"
)

//------------------------------------------------------------------------------
//
// Typedefs
//
//------------------------------------------------------------------------------

// An ExecutionEngine is used to iterate over a series of objects.
type ExecutionEngine struct {
	query      *Query
	cursor     *C.sky_cursor
	state      *C.lua_State
	header     string
	source     string
	fullSource string
	mutex      sync.Mutex
}

//------------------------------------------------------------------------------
//
// Constructor
//
//------------------------------------------------------------------------------

func NewExecutionEngine(q *Query) (*ExecutionEngine, error) {
	// Generate Lua code from query.
	source, err := q.Codegen()
	if err != nil {
		return nil, err
	}

	// Create the engine.
	e := &ExecutionEngine{query: q, source: source}

	e.mutex.Lock()
	defer e.mutex.Unlock()

	// Initialize the engine.
	if err = e.init(); err != nil {
		fmt.Printf("%s\n\n", e.FullAnnotatedSource())
		e.destroy()
		return nil, err
	}

	// Set the prefix.
	if err = e.setPrefix(e.query.Prefix); err != nil {
		e.destroy()
		return nil, err
	}

	return e, nil
}

//------------------------------------------------------------------------------
//
// Properties
//
//------------------------------------------------------------------------------

// Retrieves the source for the engine.
func (e *ExecutionEngine) Source() string {
	return e.source
}

// Retrieves the generated header for the engine.
func (e *ExecutionEngine) Header() string {
	return e.header
}

// Retrieves the full source sent to the Lua compiler.
func (e *ExecutionEngine) FullSource() string {
	return e.fullSource
}

// Retrieves the full annotated source with line numbers.
func (e *ExecutionEngine) FullAnnotatedSource() string {
	lineNumber := 1
	r, _ := regexp.Compile(`\n`)
	return "00001 " + r.ReplaceAllStringFunc(e.fullSource, func(str string) string {
		lineNumber += 1
		return fmt.Sprintf("%s%05d ", str, lineNumber)
	})
}

// SetBucket sets the bucket that this engine will iterate on.
func (e *ExecutionEngine) SetBucket(b *bolt.Bucket) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	info := b.Tx().DB().Info()
	C.bolt_cursor_init(&e.cursor.object_cursor, unsafe.Pointer(&info.Data[0]), C.size_t(info.PageSize), C.pgid(b.Root()))
}

//------------------------------------------------------------------------------
//
// Methods
//
//------------------------------------------------------------------------------

//--------------------------------------
// Lifecycle
//--------------------------------------

// Initializes the Lua context and compiles the source code.
func (e *ExecutionEngine) init() error {
	if e.state != nil {
		return nil
	}

	// Initialize the state and open the libraries.
	if e.state = C.luaL_newstate(); e.state == nil {
		return errors.New("Unable to initialize Lua context.")
	}
	C.luaL_openlibs(e.state)

	// Generate the header file.
	if err := e.generateHeader(); err != nil {
		e.destroy()
		return err
	}

	// Generate the script.
	e.fullSource = fmt.Sprintf("%v\n%v", e.header, e.source)
	// fmt.Println(e.fullSource)

	source := C.CString(e.fullSource)
	if source == nil {
		return errors.New("skyd.ExecutionEngine: Unable to allocate full source")
	}
	defer C.free(unsafe.Pointer(source))

	// Compile the script.
	if ret := C.luaL_loadstring(e.state, source); ret != 0 {
		defer e.destroy()
		errstring := C.GoString(C.lua_tolstring(e.state, -1, nil))
		return fmt.Errorf("skyd.ExecutionEngine: Syntax Error: %v", errstring)
	}

	// Run script once to initialize.
	if ret := C.lua_pcall(e.state, 0, 0, 0); ret != 0 {
		defer e.destroy()
		errstring := C.GoString(C.lua_tolstring(e.state, -1, nil))
		return fmt.Errorf("skyd.ExecutionEngine: Init Error: %v", errstring)
	}

	// Setup cursor.
	if err := e.initCursor(); err != nil {
		e.destroy()
		return err
	}

	return nil
}

// Initializes the cursor used by the script.
func (e *ExecutionEngine) initCursor() error {
	// Create the cursor.
	minPropertyId, maxPropertyId := e.query.PropertyIdentifierRange()
	e.cursor = C.sky_cursor_new((C.int32_t)(minPropertyId), (C.int32_t)(maxPropertyId))

	// Initialize the cursor from within Lua.
	functionName := C.CString("sky_init_cursor")
	if functionName == nil {
		return errors.New("skyd.ExecutionEngine: Unable to allocate function name")
	}
	defer C.free(unsafe.Pointer(functionName))

	C.lua_getfield(e.state, -10002, functionName)
	C.lua_pushlightuserdata(e.state, unsafe.Pointer(e.cursor))
	//fmt.Printf("%s\n\n", e.FullAnnotatedSource())
	if rc := C.lua_pcall(e.state, 1, 0, 0); rc != 0 {
		luaErrString := C.GoString(C.lua_tolstring(e.state, -1, nil))
		return fmt.Errorf("Unable to init cursor: %s", luaErrString)
	}

	// Set the minimum timestamps.
	if !e.query.MinTimestamp.IsZero() {
		b := db.ShiftTimeBytes(e.query.MinTimestamp)
		e.cursor.min_event_key.size = C.uint32_t(len(b))
		e.cursor.min_event_key.data = C.malloc(C.size_t(len(b)))
		C.memmove(e.cursor.min_event_key.data, unsafe.Pointer(&b[0]), C.size_t(len(b)))
	}

	// Set the maximum timestamps.
	if !e.query.MaxTimestamp.IsZero() {
		b := db.ShiftTimeBytes(e.query.MaxTimestamp)
		e.cursor.max_event_key.size = C.uint32_t(len(b))
		e.cursor.max_event_key.data = C.malloc(C.size_t(len(b)))
		C.memmove(e.cursor.max_event_key.data, unsafe.Pointer(&b[0]), C.size_t(len(b)))
	}

	return nil
}

// Closes the lua context.
func (e *ExecutionEngine) Destroy() {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.destroy()
}

func (e *ExecutionEngine) destroy() {
	if e.state != nil {
		C.lua_close(e.state)
		e.state = nil
	}
	if e.cursor != nil {
		C.sky_cursor_free(e.cursor)
		e.cursor = nil
	}
}

//--------------------------------------
// Prefix
//--------------------------------------

// Sets the prefix on the execution engine.
func (e *ExecutionEngine) setPrefix(prefix string) error {
	if e.cursor == nil {
		return errors.New("Cursor not initialized")
	}

	// Clean up existing key prefix.
	if e.cursor.key_prefix != nil {
		C.free(e.cursor.key_prefix)
		e.cursor.key_prefix = nil
		e.cursor.key_prefix_sz = 0
	}

	// Allocate new prefix.
	if prefix == "" {
		e.cursor.key_prefix = nil
	} else {
		if e.cursor.key_prefix = unsafe.Pointer(C.CString(prefix)); e.cursor.key_prefix == nil {
			return errors.New("skyd.ExecutionEngine: Unable to allocate cursor key prefix")
		}
	}
	e.cursor.key_prefix_sz = C.uint32_t(len(prefix))

	return nil
}

//--------------------------------------
// Execution
//--------------------------------------

// Initializes the data structure used for aggregation.
func (e *ExecutionEngine) Initialize() (interface{}, error) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	functionName := C.CString("sky_initialize")
	if functionName == nil {
		return nil, errors.New("skyd.ExecutionEngine: Unable to allocate initialization function name")
	}
	defer C.free(unsafe.Pointer(functionName))

	C.lua_getfield(e.state, -10002, functionName)
	C.lua_pushlightuserdata(e.state, unsafe.Pointer(e.cursor))
	if rc := C.lua_pcall(e.state, 1, 1, 0); rc != 0 {
		luaErrString := C.GoString(C.lua_tolstring(e.state, -1, nil))
		fmt.Println(e.FullAnnotatedSource())
		return nil, fmt.Errorf("skyd.ExecutionEngine: Unable to initialize: %s", luaErrString)
	}

	return e.decodeResult()
}

// Executes an aggregation over the entire database.
func (e *ExecutionEngine) Aggregate(data interface{}) (interface{}, error) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if e.state == nil {
		return nil, errors.New("skyd.ExecutionEngine: Engine destroyed")
	}
	functionName := C.CString("sky_aggregate")
	if functionName == nil {
		return nil, errors.New("skyd.ExecutionEngine: Unable to allocate aggregation function name")
	}
	defer C.free(unsafe.Pointer(functionName))

	C.lua_getfield(e.state, -10002, functionName)
	C.lua_pushlightuserdata(e.state, unsafe.Pointer(e.cursor))
	if err := e.encodeArgument(data); err != nil {
		return nil, err
	}
	rc := C.lua_pcall(e.state, 2, 1, 0)
	if rc != 0 {
		luaErrString := C.GoString(C.lua_tolstring(e.state, -1, nil))
		fmt.Println(e.FullAnnotatedSource())
		return nil, fmt.Errorf("skyd.ExecutionEngine: Unable to aggregate: %s", luaErrString)
	}

	return e.decodeResult()
}

// Executes an merge over the aggregated data.
func (e *ExecutionEngine) Merge(results interface{}, data interface{}) (interface{}, error) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if e.state == nil {
		return nil, errors.New("skyd.ExecutionEngine: Engine destroyed")
	}

	functionName := C.CString("sky_merge")
	if functionName == nil {
		return nil, errors.New("skyd.ExecutionEngine: Unable to allocate merge function name")
	}
	defer C.free(unsafe.Pointer(functionName))

	C.lua_getfield(e.state, -10002, functionName)
	if err := e.encodeArgument(results); err != nil {
		return results, err
	}
	if err := e.encodeArgument(data); err != nil {
		return results, err
	}
	if rc := C.lua_pcall(e.state, 2, 1, 0); rc != 0 {
		luaErrString := C.GoString(C.lua_tolstring(e.state, -1, nil))
		fmt.Println(e.FullAnnotatedSource())
		return results, fmt.Errorf("skyd.ExecutionEngine: Unable to merge: %s", luaErrString)
	}

	return e.decodeResult()
}

// Encodes a Go object into Msgpack and adds it to the function arguments.
func (e *ExecutionEngine) encodeArgument(value interface{}) error {
	// Encode Go object into msgpack.
	var handle codec.MsgpackHandle
	handle.RawToString = true
	buffer := new(bytes.Buffer)
	encoder := codec.NewEncoder(buffer, &handle)
	if err := encoder.Encode(value); err != nil {
		return err
	}

	// Push the msgpack data onto the Lua stack.
	data := buffer.String()
	cdata := C.CString(data)
	if cdata == nil {
		return errors.New("skyd.ExecutionEngine: Unable to allocate argument data")
	}
	defer C.free(unsafe.Pointer(cdata))
	C.lua_pushlstring(e.state, cdata, (C.size_t)(len(data)))

	// Convert the argument from msgpack into Lua.
	if rc := C.mp_unpack(e.state); rc != 1 {
		return errors.New("skyd.ExecutionEngine: Unable to msgpack encode Lua argument")
	}
	C.lua_remove(e.state, -2)

	return nil
}

// Decodes the result from a function into a Go object.
func (e *ExecutionEngine) decodeResult() (interface{}, error) {
	// Encode Lua object into msgpack.
	if rc := C.mp_pack(e.state); rc != 1 {
		return nil, errors.New("skyd.ExecutionEngine: Unable to msgpack decode Lua result")
	}
	sz := C.size_t(0)
	ptr := C.lua_tolstring(e.state, -1, (*C.size_t)(&sz))
	str := C.GoStringN(ptr, (C.int)(sz))
	C.lua_settop(e.state, -(1)-1) // lua_pop()

	// Decode msgpack into a Go object.
	var handle codec.MsgpackHandle
	handle.RawToString = true
	var ret interface{}
	decoder := codec.NewDecoder(bytes.NewBufferString(str), &handle)
	if err := decoder.Decode(&ret); err != nil {
		return nil, err
	}

	return ret, nil
}

//--------------------------------------
// Codegen
//--------------------------------------

// Generates the header for the script based on a source string.
func (e *ExecutionEngine) generateHeader() error {
	// Parse the header template.
	t := template.New("header.lua")
	t.Funcs(template.FuncMap{"structdef": variableStructDef, "metatypedef": metatypeFunctionDef, "initdescriptor": initDescriptorDef})
	if _, err := t.Parse(luaHeader); err != nil {
		return err
	}

	// Generate the template from the property references.
	var buffer bytes.Buffer
	if err := t.Execute(&buffer, e.query.Variables()); err != nil {
		return err
	}

	// Assign header
	e.header = buffer.String()

	return nil
}

func variableStructDef(args ...interface{}) string {
	if variable, ok := args[0].(*Variable); ok && !variable.IsSystemVariable() && variable.Name != "timestamp" {
		return fmt.Sprintf("%v _%v;", variable.cType(), variable.Name)
	}
	return ""
}

func metatypeFunctionDef(args ...interface{}) string {
	if variable, ok := args[0].(*Variable); ok && !variable.IsSystemVariable() {
		switch variable.DataType {
		case db.String:
			return fmt.Sprintf("%v = function(event) return ffi.string(event._%v.data, event._%v.length) end,", variable.Name, variable.Name, variable.Name)
		default:
			return fmt.Sprintf("%v = function(event) return event._%v end,", variable.Name, variable.Name)
		}
	}
	return ""
}

func initDescriptorDef(args ...interface{}) string {
	if variable, ok := args[0].(*Variable); ok && !variable.IsSystemVariable() {
		return fmt.Sprintf("cursor:set_property(%d, ffi.offsetof('sky_lua_event_t', '_%s'), ffi.sizeof('%s'), '%s')", variable.PropertyId, variable.Name, variable.cType(), variable.DataType)
	}
	return ""
}
