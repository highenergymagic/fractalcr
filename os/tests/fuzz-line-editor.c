/* libFuzzer entry point for terminal input and editor state invariants. */
#include "line-editor.h"
#include <assert.h>
#include <stdint.h>
#include <string.h>

int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size)
{
    static struct line_editor e;
    editor_init(&e);
    for(size_t i=0;i<size;i++) {
        enum edit_action action=editor_feed(&e,data[i]);
        assert(e.cursor<=e.length && e.length<LINE_CAPACITY);
        assert(strlen(e.line)==e.length);
        if(e.yank_valid) {
            assert(e.yank_start<=e.length);
            assert(e.yank_length<=e.length-e.yank_start);
        }
        if(action==EDIT_SUBMIT) { editor_record(&e); editor_clear(&e); }
    }
    return 0;
}
