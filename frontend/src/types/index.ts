export interface Identity {
  id: number
  identityKey: string
  nickname: string
  avatar: string
  createdAt: string
}

export interface Tag {
  id: number
  name: string
  postCount: number
}

export interface Supplement {
  id: number
  postId: number
  seq: number
  content: string
  status: number // 1 已发布 2 审核中 3 已驳回
  hitWords?: string
  reviewNote?: string
  createdAt: string
}

export interface Post {
  id: number
  identityId: number
  nickname: string
  avatar: string
  title: string
  content: string
  images: string[]
  status: number
  likeCount: number
  commentCount: number
  viewCount: number
  isFeatured: boolean
  liked: boolean
  isAuthor: boolean
  supplementCount: number
  supplements?: Supplement[]
  tags: Tag[]
  createdAt: string
}

export interface Comment {
  id: number
  postId: number
  identityId: number
  nickname: string
  avatar: string
  content: string
  likeCount: number
  liked: boolean
  isOp: boolean
  createdAt: string
}

export interface ReviewItem {
  id: number
  targetType: string
  targetId: number
  content: string
  status: number
  hitWords: string
  reviewNote: string
  createdAt: string
}

export interface PageResult<T> {
  items: T[]
  total: number
  page: number
  pageSize: number
}
