export interface Article {
  ID: number
  title: string
  article?: string
  image?: string
  tags?: string
}

export interface Attachment {
  id: number
  title: string
}

export interface Message {
  role: 'user' | 'assistant' | 'system'
  content: string
  attachments?: Attachment[]
}

export interface ChatSession {
  id: string
  title: string
  created_at: string
  updated_at: string
  messages: Message[]
}
