import React, { useEffect } from 'react'
import { useEditor, EditorContent } from '@tiptap/react'
import StarterKit from '@tiptap/starter-kit'
import { Bold, Italic, List, ListOrdered, Quote } from 'lucide-react'

export default function RichEditor({ value, onChange }) {
  const editor = useEditor({
    extensions: [StarterKit],
    content: value || '',
    onUpdate: ({ editor }) => {
      onChange(editor.getHTML())
    },
  })

  useEffect(() => {
    if (editor && value !== editor.getHTML()) {
      editor.commands.setContent(value || '', false)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value])

  if (!editor) return null

  const btn = (active) =>
    `p-2 rounded-md hover:bg-surface-100 ${active ? 'bg-surface-200 text-primary-700' : 'text-surface-600'}`

  return (
    <div className="flex flex-col flex-1">
      <div className="flex items-center gap-1 border-b border-surface-200 pb-2 mb-2">
        <button type="button" onClick={() => editor.chain().focus().toggleBold().run()} className={btn(editor.isActive('bold'))}>
          <Bold className="h-4 w-4" />
        </button>
        <button type="button" onClick={() => editor.chain().focus().toggleItalic().run()} className={btn(editor.isActive('italic'))}>
          <Italic className="h-4 w-4" />
        </button>
        <button type="button" onClick={() => editor.chain().focus().toggleBulletList().run()} className={btn(editor.isActive('bulletList'))}>
          <List className="h-4 w-4" />
        </button>
        <button type="button" onClick={() => editor.chain().focus().toggleOrderedList().run()} className={btn(editor.isActive('orderedList'))}>
          <ListOrdered className="h-4 w-4" />
        </button>
        <button type="button" onClick={() => editor.chain().focus().toggleBlockquote().run()} className={btn(editor.isActive('blockquote'))}>
          <Quote className="h-4 w-4" />
        </button>
      </div>
      <EditorContent
        editor={editor}
        className="flex-1 min-h-[300px] prose max-w-none text-sm focus:outline-none [&_.ProseMirror]:min-h-[300px] [&_.ProseMirror]:outline-none"
      />
    </div>
  )
}

// htmlToText derives a plain-text representation from HTML for the plain_text
// field. Uses DOMParser (inert document, no script execution) rather than
// assigning innerHTML on a live node.
export function htmlToText(html) {
  const doc = new DOMParser().parseFromString(html || '', 'text/html')
  return doc.body.textContent || ''
}
