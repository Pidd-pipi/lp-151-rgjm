import { useEffect, useState } from 'react'
import { Card, List, Button, Space, Tag, Typography, message, Table, Tabs, Modal, Input } from 'antd'
import { CheckOutlined, CloseOutlined } from '@ant-design/icons'
import { request } from '../api/client'
import type { PageResult, ReviewItem } from '../types'

const TARGET_META: Record<string, { color: string; label: string }> = {
  post: { color: 'blue', label: '帖子' },
  comment: { color: 'purple', label: '评论' },
  addendum: { color: 'gold', label: '楼主追记' },
}

export default function AdminPage() {
  const [reviews, setReviews] = useState<ReviewItem[]>([])
  const [words, setWords] = useState<{ id: number; word: string }[]>([])
  const [status, setStatus] = useState(1)
  const [rejectTarget, setRejectTarget] = useState<ReviewItem | null>(null)
  const [rejectNote, setRejectNote] = useState('')

  const loadReviews = async () => {
    try {
      const data = await request<PageResult<ReviewItem>>('get', '/admin/reviews', { page: 1, page_size: 50, status })
      setReviews(data.items)
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  const loadWords = async () => {
    try {
      const data = await request<{ id: number; word: string }[]>('get', '/admin/sensitive-words')
      setWords(data)
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  useEffect(() => {
    loadReviews()
    loadWords()
  }, [status])

  const approve = async (item: ReviewItem) => {
    try {
      await request('post', '/admin/reviews/action', { queueId: item.id, action: 'approve', note: '' })
      message.success(item.targetType === 'addendum' ? '追记已放行' : '已放行')
      loadReviews()
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  const confirmReject = async () => {
    if (!rejectTarget) return
    try {
      await request('post', '/admin/reviews/action', { queueId: rejectTarget.id, action: 'reject', note: rejectNote.trim() })
      message.success(rejectTarget.targetType === 'addendum' ? '追记已驳回，作者可另写一条' : '已屏蔽')
      setRejectTarget(null)
      setRejectNote('')
      loadReviews()
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  return (
    <Tabs
      items={[
        {
          key: 'reviews',
          label: '审核队列',
          children: (
            <Card>
              <Tabs
                size="small"
                activeKey={String(status)}
                onChange={(key) => setStatus(Number(key))}
                items={[
                  { key: '1', label: '待审核' },
                  { key: '2', label: '已放行' },
                  { key: '3', label: '已屏蔽' },
                ]}
              />
              <List
                dataSource={reviews}
                locale={{ emptyText: '暂无审核项' }}
                renderItem={(item) => {
                  const meta = TARGET_META[item.targetType] ?? { color: 'default', label: item.targetType }
                  return (
                    <List.Item
                      actions={
                        item.status === 1
                          ? [
                              <Button key="approve" type="primary" size="small" icon={<CheckOutlined />} onClick={() => approve(item)}>放行</Button>,
                              <Button key="reject" danger size="small" icon={<CloseOutlined />} onClick={() => setRejectTarget(item)}>
                                {item.targetType === 'addendum' ? '驳回' : '屏蔽'}
                              </Button>,
                            ]
                          : undefined
                      }
                    >
                      <List.Item.Meta
                        title={
                          <Space>
                            <Tag color={meta.color}>{meta.label}</Tag>
                            <Typography.Text type="secondary">#{item.targetId}</Typography.Text>
                            <Tag color="red">{item.hitWords || '敏感词'}</Tag>
                            <Typography.Text type="secondary">
                              状态: {item.status === 1 ? '待审核' : item.status === 2 ? '已放行' : '已驳回/屏蔽'}
                            </Typography.Text>
                            {item.status !== 1 && item.reviewNote && <Typography.Text type="secondary">备注: {item.reviewNote}</Typography.Text>}
                          </Space>
                        }
                        description={<Typography.Paragraph style={{ marginBottom: 0 }}>{item.content}</Typography.Paragraph>}
                      />
                    </List.Item>
                  )
                }}
              />
              <Modal
                title={rejectTarget?.targetType === 'addendum' ? '驳回楼主追记' : '屏蔽内容'}
                open={!!rejectTarget}
                onOk={confirmReject}
                onCancel={() => { setRejectTarget(null); setRejectNote('') }}
                okText="确认驳回"
                cancelText="取消"
                okButtonProps={{ danger: true }}
              >
                <Typography.Paragraph type="secondary">
                  {rejectTarget?.targetType === 'addendum'
                    ? '驳回后该追记不公开展示且不占用名额，作者可另写一条；驳回原因仅作者本人可见。'
                    : '确认屏蔽该内容？'}
                </Typography.Paragraph>
                <Input.TextArea
                  value={rejectNote}
                  onChange={(e) => setRejectNote(e.target.value.slice(0, 255))}
                  placeholder="驳回原因（可选，仅内容作者可见，最多 255 字）"
                  maxLength={255}
                  showCount
                  autoSize={{ minRows: 2, maxRows: 4 }}
                />
              </Modal>
            </Card>
          ),
        },
        {
          key: 'words',
          label: '敏感词库',
          children: (
            <Card>
              <Table
                rowKey="id"
                dataSource={words}
                pagination={false}
                columns={[
                  { title: 'ID', dataIndex: 'id', width: 80 },
                  { title: '敏感词', dataIndex: 'word' },
                ]}
              />
            </Card>
          ),
        },
      ]}
    />
  )
}
