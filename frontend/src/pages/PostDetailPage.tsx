import { useEffect, useMemo, useState } from 'react'
import { Card, Space, Typography, Button, Input, List, message, Tag, Avatar, Alert } from 'antd'
import { LikeOutlined, CommentOutlined, EyeOutlined, PlusOutlined } from '@ant-design/icons'
import { useParams, useNavigate } from 'react-router-dom'
import { request } from '../api/client'
import type { Comment, PageResult, Post, Supplement } from '../types'
import { getIdentity } from '../utils/storage'

const SUPPLEMENT_MAX = 2
const SUPPLEMENT_MAX_LENGTH = 500

function supplementStatusTag(item: Supplement) {
  if (item.status === 2) return <Tag color="orange">审核中</Tag>
  if (item.status === 3) return <Tag color="red">已驳回</Tag>
  return <Tag color="green">已发布</Tag>
}

export default function PostDetailPage() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [post, setPost] = useState<Post | null>(null)
  const [comments, setComments] = useState<Comment[]>([])
  const [commentText, setCommentText] = useState('')
  const [supplementText, setSupplementText] = useState('')
  const [supplementOpen, setSupplementOpen] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  const load = async () => {
    try {
      const postData = await request<Post>('get', `/posts/${id}`)
      setPost(postData)
      const commentData = await request<PageResult<Comment>>('get', `/posts/${id}/comments`, { page: 1, page_size: 20 })
      setComments(commentData.items)
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  useEffect(() => {
    load()
  }, [id])

  const supplements = useMemo(() => post?.supplements ?? [], [post])
  // 有效追记 = 已发布 + 审核中；已驳回不占额度
  const activeCount = supplements.filter((s) => s.status !== 3).length
  const hasPending = supplements.some((s) => s.status === 2)
  const canAddSupplement = !!post?.isAuthor && activeCount < SUPPLEMENT_MAX && !hasPending

  const likePost = async () => {
    if (!getIdentity()) {
      message.warning('请先创建匿名身份')
      return
    }
    try {
      await request('post', '/likes/toggle', { targetType: 'post', targetId: Number(id) })
      load()
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  const likeComment = async (commentId: number) => {
    if (!getIdentity()) {
      message.warning('请先创建匿名身份')
      return
    }
    try {
      await request('post', '/likes/toggle', { targetType: 'comment', targetId: commentId })
      load()
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  const submitComment = async () => {
    if (!getIdentity()) {
      message.warning('请先创建匿名身份')
      return
    }
    if (!commentText.trim()) return
    try {
      await request('post', '/comments', { postId: Number(id), content: commentText.trim() })
      setCommentText('')
      message.success('评论已发布')
      load()
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  const submitSupplement = async () => {
    const content = supplementText.trim()
    if (!content) return
    setSubmitting(true)
    try {
      const result = await request<{ blocked: boolean }>('post', `/posts/${id}/supplements`, { content })
      setSupplementText('')
      setSupplementOpen(false)
      if (result.blocked) {
        message.info('追记命中敏感词，已提交审核，通过后自动公开')
      } else {
        message.success('追记已发布')
      }
      load()
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setSubmitting(false)
    }
  }

  if (!post) return <Typography.Text>加载中...</Typography.Text>

  return (
    <div>
      <Button type="link" onClick={() => navigate(-1)}>返回</Button>
      <Card className="post-card">
        <Space align="start">
          <Avatar size={48} src={post.avatar} />
          <div>
            <Space>
              <Typography.Text strong>{post.nickname}</Typography.Text>
              <Tag color="gold">楼主</Tag>
              <Typography.Text type="secondary">{post.createdAt}</Typography.Text>
            </Space>
            {post.title && <Typography.Title level={4} style={{ margin: '8px 0' }}>{post.title}</Typography.Title>}
            <Typography.Paragraph style={{ whiteSpace: 'pre-wrap' }}>{post.content}</Typography.Paragraph>
            {post.images?.length > 0 && (
              <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', margin: '8px 0' }}>
                {post.images.map((url, idx) => (
                  <img key={idx} src={url} alt={`${post.nickname}-${idx}`} style={{ width: 160, height: 160, objectFit: 'cover', borderRadius: 8 }} />
                ))}
              </div>
            )}
            {post.tags.map((tag) => (
              <Tag key={tag.id} color="blue">{tag.name}</Tag>
            ))}
            <Space size="large" style={{ marginTop: 8 }}>
              <Button type={post.liked ? 'primary' : 'text'} icon={<LikeOutlined />} onClick={likePost}>
                {post.likeCount}
              </Button>
              <Typography.Text type="secondary"><CommentOutlined /> {post.commentCount}</Typography.Text>
              <Typography.Text type="secondary"><EyeOutlined /> {post.viewCount}</Typography.Text>
            </Space>
          </div>
        </Space>
      </Card>

      <Card
        title={`楼主追记 (${post.supplementCount})`}
        style={{ marginTop: 16 }}
        extra={
          canAddSupplement && !supplementOpen ? (
            <Button size="small" icon={<PlusOutlined />} onClick={() => setSupplementOpen(true)}>
              追加追记
            </Button>
          ) : null
        }
      >
        {supplements.length === 0 && !supplementOpen && (
          <Typography.Text type="secondary">暂无追记</Typography.Text>
        )}
        <List
          dataSource={supplements}
          renderItem={(item) => (
            <List.Item>
              <List.Item.Meta
                title={
                  <Space>
                    <Tag color="gold">楼主</Tag>
                    <Typography.Text strong>{item.status === 3 ? '追记（已驳回）' : `追记 #${item.seq}`}</Typography.Text>
                    {post.isAuthor && supplementStatusTag(item)}
                    <Typography.Text type="secondary">{item.createdAt}</Typography.Text>
                  </Space>
                }
                description={
                  <div>
                    <Typography.Paragraph style={{ marginBottom: 0, whiteSpace: 'pre-wrap' }}>{item.content}</Typography.Paragraph>
                    {post.isAuthor && item.status === 3 && item.reviewNote && (
                      <Alert style={{ marginTop: 8 }} type="error" showIcon message={`驳回原因：${item.reviewNote}`} />
                    )}
                    {post.isAuthor && item.status === 2 && (
                      <Typography.Text type="warning">审核通过后自动公开，期间仅自己可见</Typography.Text>
                    )}
                  </div>
                }
              />
            </List.Item>
          )}
        />
        {supplementOpen && (
          <div style={{ marginTop: 8 }}>
            <Input.TextArea
              value={supplementText}
              onChange={(e) => setSupplementText(e.target.value)}
              placeholder="补充说明（提交后不可修改，最多 500 字）"
              autoSize={{ minRows: 2, maxRows: 6 }}
              maxLength={SUPPLEMENT_MAX_LENGTH}
              showCount
            />
            <Space style={{ marginTop: 8 }}>
              <Button type="primary" loading={submitting} onClick={submitSupplement}>
                提交追记
              </Button>
              <Button onClick={() => setSupplementOpen(false)}>取消</Button>
            </Space>
          </div>
        )}
        {post.isAuthor && !canAddSupplement && !supplementOpen && (
          <Typography.Text type="secondary" style={{ display: 'block', marginTop: 8 }}>
            {hasPending ? '已有追记正在审核，审核完成后可继续追加' : activeCount >= SUPPLEMENT_MAX ? '追记已达上限（2 条）' : ''}
          </Typography.Text>
        )}
      </Card>

      <Card title={`评论 (${comments.length})`} style={{ marginTop: 16 }}>
        <Space.Compact style={{ width: '100%', marginBottom: 16 }}>
          <Input.TextArea value={commentText} onChange={(e) => setCommentText(e.target.value)} placeholder="写下你的匿名评论..." autoSize={{ minRows: 2, maxRows: 4 }} />
        </Space.Compact>
        <Button type="primary" onClick={submitComment} style={{ marginTop: 8 }}>发表评论</Button>
        <List
          style={{ marginTop: 16 }}
          dataSource={comments}
          locale={{ emptyText: '暂无评论' }}
          renderItem={(item) => (
            <List.Item
              actions={[
                <Button key="like" type="text" icon={<LikeOutlined />} onClick={() => likeComment(item.id)}>
                  {item.likeCount}
                </Button>,
              ]}
            >
              <List.Item.Meta
                avatar={<Avatar src={item.avatar} />}
                title={
                  <Space>
                    <Typography.Text strong>{item.nickname}</Typography.Text>
                    {item.isOp && <Tag color="gold">楼主</Tag>}
                    <Typography.Text type="secondary">{item.createdAt}</Typography.Text>
                  </Space>
                }
                description={<Typography.Paragraph style={{ marginBottom: 0 }}>{item.content}</Typography.Paragraph>}
              />
            </List.Item>
          )}
        />
      </Card>
    </div>
  )
}
