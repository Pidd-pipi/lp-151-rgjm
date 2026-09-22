import { useEffect, useState } from 'react'
import { Card, Space, Typography, Button, Input, List, message, Tag, Avatar, Alert, Badge } from 'antd'
import { LikeOutlined, CommentOutlined, EyeOutlined, EditOutlined } from '@ant-design/icons'
import { useParams, useNavigate } from 'react-router-dom'
import { request } from '../api/client'
import type { Comment, PageResult, Post, Addendum } from '../types'
import { getIdentity } from '../utils/storage'

const ADDENDUM_STATUS_PUBLISHED = 1
const ADDENDUM_STATUS_PENDING = 2
const ADDENDUM_STATUS_REJECTED = 3

export default function PostDetailPage() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [post, setPost] = useState<Post | null>(null)
  const [comments, setComments] = useState<Comment[]>([])
  const [commentText, setCommentText] = useState('')
  const [addendumText, setAddendumText] = useState('')
  const [submittingAddendum, setSubmittingAddendum] = useState(false)

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

  const submitAddendum = async () => {
    const identity = getIdentity()
    if (!identity) {
      message.warning('请先创建匿名身份')
      return
    }
    const content = addendumText.trim()
    if (!content) return
    if (content.length > 500) {
      message.warning('追记不能超过 500 字')
      return
    }
    setSubmittingAddendum(true)
    try {
      const data = await request<{ addendum: Addendum; blocked: boolean }>('post', `/posts/${id}/addenda`, { content })
      setAddendumText('')
      message.success(data.blocked ? '追记命中敏感词，已进入审核，通过后将公开展示' : '追记已发布')
      load()
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setSubmittingAddendum(false)
    }
  }

  const renderAddendumStatus = (item: Addendum) => {
    if (item.status === ADDENDUM_STATUS_PENDING) {
      return <Tag color="orange">审核中</Tag>
    }
    if (item.status === ADDENDUM_STATUS_REJECTED) {
      return <Tag color="red">已驳回</Tag>
    }
    return <Tag color="green">已通过</Tag>
  }

  if (!post) return <Typography.Text>加载中...</Typography.Text>

  const identity = getIdentity()
  const isOwner = !!identity && post.isOwner
  const addenda = post.addenda ?? []
  const hasPending = addenda.some((a) => a.status === ADDENDUM_STATUS_PENDING)
  // 名额口径与后端一致：已通过 + 待审最多 2 条，驳回不占名额
  const occupiedSlots = addenda.filter((a) => a.status === ADDENDUM_STATUS_PUBLISHED || a.status === ADDENDUM_STATUS_PENDING).length
  const canSubmitAddendum = isOwner && occupiedSlots < 2 && !hasPending

  return (
    <div>
      <Button type="link" onClick={() => navigate(-1)}>返回</Button>
      <Card className="post-card">
        <Space align="start">
          <Avatar size={48} src={post.avatar} />
          <div>
            <Space>
              <Typography.Text strong>{post.nickname}</Typography.Text>
              {isOwner && <Tag color="gold">楼主</Tag>}
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
              <Badge count={post.addendumCount} size="small" offset={[2, 0]}>
                <Typography.Text type="secondary"><EditOutlined /> 楼主追记</Typography.Text>
              </Badge>
            </Space>
          </div>
        </Space>
      </Card>

      {(addenda.length > 0 || isOwner) && (
        <Card
          title={<Space><EditOutlined />楼主追记{post.addendumCount > 0 && <Tag>{post.addendumCount} 条</Tag>}</Space>}
          style={{ marginTop: 16 }}
        >
          <List
            dataSource={addenda}
            locale={{ emptyText: '暂无追记' }}
            renderItem={(item) => (
              <List.Item style={{ display: 'block' }}>
                <Space style={{ marginBottom: 4 }}>
                  <Tag color="gold">楼主</Tag>
                  {renderAddendumStatus(item)}
                  <Typography.Text type="secondary">{item.createdAt}</Typography.Text>
                </Space>
                <Typography.Paragraph style={{ whiteSpace: 'pre-wrap', marginBottom: 4 }}>{item.content}</Typography.Paragraph>
                {item.status === ADDENDUM_STATUS_PENDING && (
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    命中敏感词{item.hitWords ? `（${item.hitWords}）` : ''}，正在等待管理员审核；审核期间原帖与已通过的追记仍公开展示。
                  </Typography.Text>
                )}
                {item.status === ADDENDUM_STATUS_REJECTED && (
                  <Alert
                    style={{ marginTop: 4 }}
                    type="error"
                    showIcon
                    message={
                      <Space direction="vertical" size={0}>
                        <Typography.Text type="danger" strong>追记未通过审核</Typography.Text>
                        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                          驳回原因（仅你可见）：{item.reviewNote || '未填写'}。被驳回不占名额，你可以修改后重新追加一条。
                        </Typography.Text>
                      </Space>
                    }
                  />
                )}
              </List.Item>
            )}
          />
          {isOwner && (
            <div style={{ marginTop: 12 }}>
              {hasPending && <Alert style={{ marginBottom: 8 }} type="warning" showIcon message="你已有一条追记正在审核，审核完成前不能追加新的追记。" />}
              {occupiedSlots >= 2 && !hasPending && <Alert style={{ marginBottom: 8 }} type="info" showIcon message="该帖两条追记名额已用完，追记发布后不可修改。" />}
              <Input.TextArea
                value={addendumText}
                onChange={(e) => setAddendumText(e.target.value.slice(0, 500))}
                placeholder={canSubmitAddendum ? '以楼主身份追加补充说明（发布后不可修改，最多 500 字、最多 2 条）' : '暂不可追加追记'}
                disabled={!canSubmitAddendum}
                maxLength={500}
                showCount
                autoSize={{ minRows: 2, maxRows: 4 }}
              />
              <Button
                type="primary"
                style={{ marginTop: 8 }}
                loading={submittingAddendum}
                disabled={!canSubmitAddendum || !addendumText.trim()}
                onClick={submitAddendum}
              >
                追加追记
              </Button>
            </div>
          )}
        </Card>
      )}

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
                title={<Space><Typography.Text strong>{item.nickname}</Typography.Text><Typography.Text type="secondary">{item.createdAt}</Typography.Text></Space>}
                description={<Typography.Paragraph style={{ marginBottom: 0 }}>{item.content}</Typography.Paragraph>}
              />
            </List.Item>
          )}
        />
      </Card>
    </div>
  )
}
