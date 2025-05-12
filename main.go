package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"sort"

	_ "github.com/go-sql-driver/mysql"
)

// Channel 表示数据库中的通道记录
type Channel struct {
	ID               int
	Status           string
	CountMinuteUsage int
	CountDayUsage    int
	Tag              string
}

// ChannelView 表示前端展示的通道视图
type ChannelView struct {
	ID               int
	StatusDisplay    string
	CountMinuteUsage int
	CountDayUsage    int
	TagDisplay       string
	MinuteLimit      int
	DayLimit         int
	MinutePercentage float64
	DayPercentage    float64
	IsPaid           bool // 用于排序
	IsAvailable      bool // 用于统计可用普号数量
}

// SummaryData 表示总体使用情况摘要
type SummaryData struct {
	TotalMinuteUsage          int
	TotalDayUsage             int
	TotalMinuteLimit          int
	TotalDayLimit             int
	MinutePercentage          float64
	DayPercentage             float64
	DisabledNormalChannels    int     // 自动禁用普号数
	TotalNormalChannels       int
	DisabledNormalPercentage  float64 // 自动禁用普号百分比
}

// 从环境变量获取配置，如果不存在则使用默认值
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func main() {
	// 从环境变量获取数据库配置
	dbUser := getEnv("DB_USER", "root")
	dbPassword := getEnv("DB_PASSWORD", "")
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "3306")
	dbName := getEnv("DB_NAME", "gemini")
	serverPort := getEnv("SERVER_PORT", "8080")

	// 构建数据库连接字符串
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s",
		dbUser,
		dbPassword,
		dbHost,
		dbPort,
		dbName)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("无法连接到数据库: %v", err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		log.Fatalf("数据库连接测试失败: %v", err)
	}
	log.Println("数据库连接测试成功")

	// 处理主页请求
	log.Println("注册主页处理函数...")
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query("SELECT id, status, count_minute_usage, count_day_usage, tag FROM channels")
		if err != nil {
			http.Error(w, "查询数据库失败", http.StatusInternalServerError)
			log.Printf("查询失败: %v", err)
			return
		}
		defer rows.Close()

		var channelViews []ChannelView
		var summary SummaryData
		availableNormalChannels := 0

		for rows.Next() {
			var channel Channel
			if err := rows.Scan(&channel.ID, &channel.Status, &channel.CountMinuteUsage, &channel.CountDayUsage, &channel.Tag); err != nil {
				http.Error(w, "处理查询结果失败", http.StatusInternalServerError)
				log.Printf("扫描行数据失败: %v", err)
				return
			}

			view := ChannelView{
				ID:               channel.ID,
				CountMinuteUsage: channel.CountMinuteUsage,
				CountDayUsage:    channel.CountDayUsage,
			}

			// 根据SQL脚本逻辑调整：status 1 为可用，其他（包括 2）为自动禁用
			if channel.Status == "1" {
				view.StatusDisplay = "可用"
				view.IsAvailable = true
			} else {
				view.StatusDisplay = "自动禁用" // 包括 status 2 或其他非 1 的值
				view.IsAvailable = false
			}

			if channel.Tag == "gcp" {
				view.TagDisplay = "付费号"
				view.MinuteLimit = 20
				view.DayLimit = 100
				view.IsPaid = true
			} else {
				view.TagDisplay = "普号"
				view.MinuteLimit = 5
				view.DayLimit = 25
				view.IsPaid = false
				summary.TotalNormalChannels++
				if view.IsAvailable {
					availableNormalChannels++
				}
			}

			if view.MinuteLimit > 0 {
				view.MinutePercentage = float64(channel.CountMinuteUsage) / float64(view.MinuteLimit) * 100
				if view.MinutePercentage > 100 {
					view.MinutePercentage = 100
				}
			} else {
				view.MinutePercentage = 0
			}

			if view.DayLimit > 0 {
				view.DayPercentage = float64(channel.CountDayUsage) / float64(view.DayLimit) * 100
				if view.DayPercentage > 100 {
					view.DayPercentage = 100
				}
			} else {
				view.DayPercentage = 0
			}

			summary.TotalMinuteUsage += channel.CountMinuteUsage
			summary.TotalDayUsage += channel.CountDayUsage
			summary.TotalMinuteLimit += view.MinuteLimit
			summary.TotalDayLimit += view.DayLimit

			channelViews = append(channelViews, view)
		}

		summary.DisabledNormalChannels = summary.TotalNormalChannels - availableNormalChannels

		if summary.TotalMinuteLimit > 0 {
			summary.MinutePercentage = float64(summary.TotalMinuteUsage) / float64(summary.TotalMinuteLimit) * 100
			if summary.MinutePercentage > 100 {
				summary.MinutePercentage = 100
			}
		}
		if summary.TotalDayLimit > 0 {
			summary.DayPercentage = float64(summary.TotalDayUsage) / float64(summary.TotalDayLimit) * 100
			if summary.DayPercentage > 100 {
				summary.DayPercentage = 100
			}
		}

		if summary.TotalNormalChannels > 0 {
			summary.DisabledNormalPercentage = float64(summary.DisabledNormalChannels) / float64(summary.TotalNormalChannels) * 100
		}

		if err = rows.Err(); err != nil {
			http.Error(w, "查询过程中发生错误", http.StatusInternalServerError)
			log.Printf("查询过程错误: %v", err)
			return
		}

		sort.SliceStable(channelViews, func(i, j int) bool {
			if channelViews[i].IsPaid != channelViews[j].IsPaid {
				return channelViews[i].IsPaid
			}
			if channelViews[i].DayPercentage != channelViews[j].DayPercentage {
				return channelViews[i].DayPercentage < channelViews[j].DayPercentage
			}
			return channelViews[i].MinutePercentage < channelViews[j].MinutePercentage
		})

		data := struct {
			Channels []ChannelView
			Summary  SummaryData
		}{
			Channels: channelViews,
			Summary:  summary,
		}

		// HTML模板
		tmpl := `
<!DOCTYPE html>
<html>
<head>
    <title>Gemini 2.5 Pro监控</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            margin: 20px;
            background-color: #f5f7fa;
        }
        h1 {
            text-align: center;
            margin-bottom: 30px;
            color: #333;
        }
        .container {
            max-width: 1400px;
            margin: 0 auto;
        }
        .summary-card {
            width: 100%;
            border-radius: 10px;
            padding: 20px;
            margin-bottom: 30px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
            background-color: white;
            box-sizing: border-box;
        }
        .summary-title {
            font-size: 22px;
            font-weight: bold;
            margin-bottom: 20px;
            color: #333;
            text-align: center;
        }
        .progress-container {
            width: 100%;
            background-color: #e0e0e0;
            border-radius: 4px;
            margin: 10px 0;
            height: 20px;
            position: relative;
            overflow: hidden;
        }
        .progress-bar {
            height: 100%;
            border-radius: 4px;
            position: relative;
            display: flex;
            align-items: center; /* Vertically center */
            justify-content: center; /* Horizontally center */
            box-sizing: border-box;
            color: white;
            font-size: 12px;
            font-weight: bold;
            text-shadow: 0 0 3px rgba(0,0,0,0.5);
            min-width: 2%; /* Keep this for visibility of small percentages */
            white-space: nowrap; /* Keep this to prevent wrapping */
        }
        .control-panel {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 20px;
            flex-wrap: wrap;
            gap: 10px;
        }
        .search-box {
            padding: 8px 15px;
            border: 1px solid #ddd;
            border-radius: 20px;
            width: 250px;
            box-sizing: border-box;
        }
        .filter-group {
            display: flex;
            gap: 10px;
            flex-wrap: wrap;
        }
        .filter-btn {
            padding: 8px 15px;
            background: white;
            border: 1px solid #ddd;
            border-radius: 20px;
            cursor: pointer;
            white-space: nowrap;
        }
        .filter-btn.active {
            background: #4285f4;
            color: white;
            border-color: #4285f4;
        }
        .cards-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(250px, 1fr));
            gap: 20px;
        }
        .channel-card {
            background: white;
            border-radius: 10px;
            overflow: hidden;
            box-shadow: 0 2px 8px rgba(0,0,0,0.08);
            transition: transform 0.2s;
            position: relative;
            display: flex;
            flex-direction: column;
        }
        .channel-card:hover {
            transform: translateY(-3px);
        }
        .channel-header {
            padding: 10px 15px;
            border-bottom: 1px solid #eee;
            display: flex;
            align-items: center;
            gap: 8px;
            flex-shrink: 0;
        }
        .channel-id {
            font-weight: bold;
            font-size: 16px;
            white-space: nowrap;
            overflow: hidden;
            text-overflow: ellipsis;
            flex-shrink: 0;
        }
        .tag-badge-center {
            margin-left: auto;
            margin-right: auto;
        }
        .status-badge {
            padding: 3px 7px;
            border-radius: 10px;
            font-size: 13px;
            font-weight: bold;
            white-space: nowrap;
            flex-shrink: 0;
            line-height: 1.2;
        }
        .channel-body {
            padding: 15px;
            flex-grow: 1;
            display: flex;
            flex-direction: column;
            justify-content: space-around;
        }
        .usage-label {
            display: flex;
            justify-content: space-between;
            margin-bottom: 5px;
            font-size: 13px;
            color: #666;
        }
        /* Badge color styles remain the same */
        .status-available {
            background-color: #e6f4ea;
            color: #137333;
        }
        .status-unavailable {
            background-color: #fce8e6;
            color: #c5221f;
        }
        .tag-paid {
            background-color: #e8f0fe;
            color: #1a73e8;
        }
        .tag-normal {
            background-color: #f1f3f4;
            color: #5f6368;
        }

        .summary-card .usage-label span {
            font-size: 1.1em;
            font-weight: bold;
        }

        /* Responsive adjustments */
        @media (max-width: 768px) {
            .container { margin: 10px; }
            .cards-grid { grid-template-columns: repeat(auto-fill, minmax(220px, 1fr)); gap: 15px; }
            .control-panel { flex-direction: column; align-items: stretch; }
            .search-box { width: 100%; }
            .filter-group { justify-content: center; }
            h1 { font-size: 24px; margin-bottom: 20px; }
            .summary-card { padding: 15px; }
            .summary-title { font-size: 20px; }
            .summary-card .usage-label span { font-size: 1.05em; }
            .channel-header { padding: 8px 12px; gap: 6px; }
            .channel-id { font-size: 15px; }
            .status-badge {
                 font-size: 12px;
                 padding: 2px 6px;
            }
            .progress-bar {
                font-size: 11px; /* Slightly smaller font for smaller screens */
            }
        }
         @media (max-width: 480px) {
             .cards-grid { grid-template-columns: 1fr; }
             .channel-card { min-width: 0; }
             .filter-btn { padding: 6px 12px; font-size: 13px; }
             .channel-id { font-size: 14px; }
             .status-badge {
                font-size: 11px;
                padding: 2px 5px;
             }
             .usage-label { font-size: 12px; }
             .progress-bar {
                 font-size: 10px; /* Even smaller font for very small screens */
             }
             .summary-card .usage-label span { font-size: 1em; }
             .channel-header { padding: 6px 10px; gap: 5px; }
         }
    </style>
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body>
    <div class="container">
        <h1>Gemini 2.5 Pro监控</h1>

        <!-- 总使用情况 -->
        <div class="summary-card">
            <div class="summary-title">总使用情况</div>
            <div class="summary-container">
                <!-- Minute Usage -->
                <div class="summary-progress">
                    <div class="usage-label">
                        <span>过去一分钟总使用次数：</span>
                        <span>{{.Summary.TotalMinuteUsage}} / {{.Summary.TotalMinuteLimit}}</span>
                    </div>
                    <div class="progress-container">
                        <div class="progress-bar" style="width: {{printf "%.1f" .Summary.MinutePercentage}}%; background-color: {{if gt .Summary.MinutePercentage 80.0}}#ff4d4d{{else if gt .Summary.MinutePercentage 50.0}}#ffa64d{{else}}#4CAF50{{end}};">
                            {{printf "%.1f" .Summary.MinutePercentage}}%
                        </div>
                    </div>
                </div>
                <!-- Day Usage -->
                <div class="summary-progress">
                    <div class="usage-label">
                        <span>过去一天总使用次数（早上8点重置）：</span>
                        <span>{{.Summary.TotalDayUsage}} / {{.Summary.TotalDayLimit}}</span>
                    </div>
                    <div class="progress-container">
                        <div class="progress-bar" style="width: {{printf "%.1f" .Summary.DayPercentage}}%; background-color: {{if gt .Summary.DayPercentage 80.0}}#ff4d4d{{else if gt .Summary.DayPercentage 50.0}}#ffa64d{{else}}#4CAF50{{end}};">
                            {{printf "%.1f" .Summary.DayPercentage}}%
                        </div>
                    </div>
                </div>
                 <!-- Disabled Normal Channels -->
                 <div class="summary-progress">
                    <div class="usage-label">
                        <span>自动禁用普号数：</span>
                        <span>{{.Summary.DisabledNormalChannels}} / {{.Summary.TotalNormalChannels}}</span>
                    </div>
                    <div class="progress-container">
                        <div class="progress-bar" style="width: {{printf "%.1f" .Summary.DisabledNormalPercentage}}%; background-color: {{if gt .Summary.DisabledNormalPercentage 80.0}}#ff4d4d{{else if gt .Summary.DisabledNormalPercentage 50.0}}#ffa64d{{else}}#4CAF50{{end}};">
                            {{printf "%.1f" .Summary.DisabledNormalPercentage}}%
                        </div>
                    </div>
                </div>
            </div>
        </div>

        <!-- 控制面板 -->
        <div class="control-panel">
            <input type="text" class="search-box" placeholder="搜索ID..." id="searchInput">
            <div class="filter-group">
                <button class="filter-btn active" data-filter="all">全部</button>
                <button class="filter-btn" data-filter="available">可用</button>
                <button class="filter-btn" data-filter="unavailable">自动禁用</button>
                <button class="filter-btn" data-filter="paid">付费号</button>
                <button class="filter-btn" data-filter="normal">普号</button>
            </div>
        </div>
        <!-- 卡片网格 -->
        <div class="cards-grid" id="channelsGrid">
            {{range .Channels}}
            <div class="channel-card"
                 data-id="{{.ID}}"
                 data-status="{{if eq .StatusDisplay "可用"}}available{{else}}unavailable{{end}}"
                 data-type="{{if eq .TagDisplay "付费号"}}paid{{else}}normal{{end}}">
                <!-- Header -->
                <div class="channel-header">
                    <div class="channel-id">ID: {{.ID}}</div>
                    <span class="status-badge tag-badge-center {{if eq .TagDisplay "付费号"}}tag-paid{{else}}tag-normal{{end}}">
                        {{.TagDisplay}}
                    </span>
                    <span class="status-badge {{if eq .StatusDisplay "可用"}}status-available{{else}}status-unavailable{{end}}">
                        {{.StatusDisplay}}
                    </span>
                </div>
                <!-- Body -->
                <div class="channel-body">
                    <!-- Minute Usage -->
                    <div>
                        <div class="usage-label">
                            <span>过去一分钟：</span>
                            <span>{{.CountMinuteUsage}} / {{.MinuteLimit}}</span>
                        </div>
                        <div class="progress-container">
                            <div class="progress-bar" style="width: {{printf "%.1f" .MinutePercentage}}%; background-color: {{if gt .MinutePercentage 80.0}}#ff4d4d{{else if gt .MinutePercentage 50.0}}#ffa64d{{else}}#4CAF50{{end}};">
                                {{printf "%.1f" .MinutePercentage}}%
                            </div>
                        </div>
                    </div>
                    <!-- Day Usage -->
                    <div>
                        <div class="usage-label">
                            <span>过去一天：</span>
                            <span>{{.CountDayUsage}} / {{.DayLimit}}</span>
                        </div>
                        <div class="progress-container">
                            <div class="progress-bar" style="width: {{printf "%.1f" .DayPercentage}}%; background-color: {{if gt .DayPercentage 80.0}}#ff4d4d{{else if gt .DayPercentage 50.0}}#ffa64d{{else}}#4CAF50{{end}};">
                                {{printf "%.1f" .DayPercentage}}%
                            </div>
                        </div>
                    </div>
                </div>
            </div>
            {{end}}
        </div>
    </div>
    <script>
     document.addEventListener('DOMContentLoaded', function() {
        const searchInput = document.getElementById('searchInput');
        const filterButtons = document.querySelectorAll('.filter-btn');
        const channelCards = document.querySelectorAll('.channel-card');
        let currentFilter = 'all';

        function filterChannels(searchTerm, filter) {
            channelCards.forEach(card => {
                const id = card.getAttribute('data-id').toLowerCase();
                const status = card.getAttribute('data-status');
                const type = card.getAttribute('data-type');

                const matchesSearch = searchTerm === '' || id.includes(searchTerm);

                let matchesFilter = false;
                if (filter === 'all') {
                    matchesFilter = true;
                } else if (filter === 'available' || filter === 'unavailable') {
                    matchesFilter = status === filter;
                } else if (filter === 'paid' || filter === 'normal') {
                    matchesFilter = type === filter;
                }

                if (matchesSearch && matchesFilter) {
                    card.style.display = ''; // Use default display (grid item)
                } else {
                    card.style.display = 'none';
                }
            });
        }

        searchInput.addEventListener('input', function() {
            const searchTerm = this.value.toLowerCase().trim();
            filterChannels(searchTerm, currentFilter);
        });

        filterButtons.forEach(button => {
            button.addEventListener('click', function() {
                const filter = this.getAttribute('data-filter');
                if (currentFilter !== filter) {
                    currentFilter = filter;
                    filterButtons.forEach(btn => btn.classList.remove('active'));
                    this.classList.add('active');
                    filterChannels(searchInput.value.toLowerCase().trim(), filter);
                }
            });
        });

        // Initial filter on load
        filterChannels(searchInput.value.toLowerCase().trim(), currentFilter);

        // Auto-refresh
        setTimeout(function() {
            location.reload();
        }, 60000); // Refresh every 60 seconds
    });
    </script>
</body>
</html>
`

		t, err := template.New("channels").Funcs(template.FuncMap{}).Parse(tmpl)
		if err != nil {
			http.Error(w, "模板解析失败", http.StatusInternalServerError)
			log.Printf("模板解析失败: %v", err)
			return
		}
		err = t.Execute(w, data)
		if err != nil {
			http.Error(w, "模板执行失败", http.StatusInternalServerError)
			log.Printf("模板执行失败: %v", err)
			return
		}
	})

	// 启动服务器
	log.Printf("尝试在 0.0.0.0:%s 上启动服务器...", serverPort)
	err = http.ListenAndServe(":"+serverPort, nil)
	if err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}
