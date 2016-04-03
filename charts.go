// Known issues: hashrate is overestimated for 1h

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/callum-ramage/jsonconfig"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Change to 0 to connect to remote host
var localhost_for_daemon_rpc = 1

type CoinHeight struct {
	Height int    `json:"height"`
	Status string `json:"status"`
}

type CoinConfig struct {
	Jsonrpc string `json:"jsonrpc"`
	Result  struct {
		BaseCoin struct {
			Git  string `json:"git"`
			Name string `json:"name"`
		} `json:"base_coin"`
		Core struct {
			BYTECOINNETWORK                        string   `json:"BYTECOIN_NETWORK"`
			CHECKPOINTS                            []string `json:"CHECKPOINTS"`
			CRYPTONOTEBLOCKGRANTEDFULLREWARDZONE   int      `json:"CRYPTONOTE_BLOCK_GRANTED_FULL_REWARD_ZONE"`
			CRYPTONOTEBLOCKGRANTEDFULLREWARDZONEV1 int      `json:"CRYPTONOTE_BLOCK_GRANTED_FULL_REWARD_ZONE_V1"`
			CRYPTONOTEDISPLAYDECIMALPOINT          int      `json:"CRYPTONOTE_DISPLAY_DECIMAL_POINT"`
			CRYPTONOTEMINEDMONEYUNLOCKWINDOW       int      `json:"CRYPTONOTE_MINED_MONEY_UNLOCK_WINDOW"`
			CRYPTONOTENAME                         string   `json:"CRYPTONOTE_NAME"`
			CRYPTONOTEPUBLICADDRESSBASE58PREFIX    int      `json:"CRYPTONOTE_PUBLIC_ADDRESS_BASE58_PREFIX"`
			DEFAULTDUSTTHRESHOLD                   int      `json:"DEFAULT_DUST_THRESHOLD"`
			DIFFICULTYCUT                          int      `json:"DIFFICULTY_CUT"`
			DIFFICULTYLAG                          int      `json:"DIFFICULTY_LAG"`
			DIFFICULTYTARGET                       int      `json:"DIFFICULTY_TARGET"`
			EMISSIONSPEEDFACTOR                    int      `json:"EMISSION_SPEED_FACTOR"`
			EXPECTEDNUMBEROFBLOCKSPERDAY           int      `json:"EXPECTED_NUMBER_OF_BLOCKS_PER_DAY"`
			GENESISBLOCKREWARD                     int      `json:"GENESIS_BLOCK_REWARD"`
			GENESISCOINBASETXHEX                   string   `json:"GENESIS_COINBASE_TX_HEX"`
			MAXBLOCKSIZEINITIAL                    int      `json:"MAX_BLOCK_SIZE_INITIAL"`
			MINIMUMFEE                             int      `json:"MINIMUM_FEE"`
			MONEYSUPPLY                            string   `json:"MONEY_SUPPLY"`
			P2PDEFAULTPORT                         int      `json:"P2P_DEFAULT_PORT"`
			RPCDEFAULTPORT                         int      `json:"RPC_DEFAULT_PORT"`
			SEEDNODES                              []string `json:"SEED_NODES"`
			UPGRADEHEIGHT                          int      `json:"UPGRADE_HEIGHT"`
		} `json:"core"`
		Extensions []string `json:"extensions"`
		Status     string   `json:"status"`
	} `json:"result"`
}

type Block struct {
	ID      string `json:"id"`
	Jsonrpc string `json:"jsonrpc"`
	Result  struct {
		Block struct {
			AlreadyGeneratedCoins        string  `json:"alreadyGeneratedCoins"`
			AlreadyGeneratedTransactions int     `json:"alreadyGeneratedTransactions"`
			BaseReward                   int     `json:"baseReward"`
			BlockSize                    int     `json:"blockSize"`
			Depth                        int     `json:"depth"`
			Difficulty                   int     `json:"difficulty"`
			EffectiveSizeMedian          int     `json:"effectiveSizeMedian"`
			Hash                         string  `json:"hash"`
			Height                       int     `json:"height"`
			MajorVersion                 int     `json:"major_version"`
			MinorVersion                 int     `json:"minor_version"`
			Nonce                        int     `json:"nonce"`
			OrphanStatus                 bool    `json:"orphan_status"`
			Penalty                      float64 `json:"penalty"`
			PrevHash                     string  `json:"prev_hash"`
			Reward                       int     `json:"reward"`
			SizeMedian                   int     `json:"sizeMedian"`
			Timestamp                    int     `json:"timestamp"`
			TotalFeeAmount               int     `json:"totalFeeAmount"`
			Transactions                 []struct {
				AmountOut int    `json:"amount_out"`
				Fee       int    `json:"fee"`
				Hash      string `json:"hash"`
				Size      int    `json:"size"`
			} `json:"transactions"`
			TransactionsCumulativeSize int `json:"transactionsCumulativeSize"`
		} `json:"block"`
		Status string `json:"status"`
	} `json:"result"`
}

type Chart [][]uint64

type ChartInterval struct {
	Suffix               string
	ExpectedTimePerCycle uint64
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func readFile(path string) []byte {
	_, err := os.Stat(path)
	if err != nil {
		writeFile(path, []byte(""))
	}

	dat, err := ioutil.ReadFile(path)
	check(err)
	return dat
}

func writeFile(path string, content []byte) {
	os.MkdirAll(filepath.Dir(path), 0775)
	err := ioutil.WriteFile(path, content, 0644)
	check(err)
}

func readChart(filename string) Chart {
	chart_bytes := readFile(filename)
	if len(chart_bytes) == 0 {
		chart_bytes = []byte("[]")
	}
	var chart Chart
	err := json.Unmarshal(chart_bytes, &chart)
	check(err)
	return chart
}

func addPointToChart(point1 int, point2 uint64, chart Chart, filename string) Chart {
	point := []uint64{}
	point = append(point, uint64(point1))
	point = append(point, point2)
	chart = append(chart, point)
	writeChart(chart, filename)
	return chart
}

func writeChart(chart Chart, filename string) {
	chart_bytes, err := json.Marshal(chart)
	check(err)
	writeFile(filename, chart_bytes)
}

func main() {

	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	check(err)

	var chartIntervals []ChartInterval
	newChartInterval := ChartInterval{Suffix: "_1d", ExpectedTimePerCycle: 24 * 60 * 60}
	chartIntervals = append(chartIntervals, newChartInterval)
	newChartInterval = ChartInterval{Suffix: "_1h", ExpectedTimePerCycle: 1 * 60 * 60}
	chartIntervals = append(chartIntervals, newChartInterval)

	pools, err := jsonconfig.LoadAbstract(dir+"/pools.json", "")

	for _, chartInterval := range chartIntervals {
		var expected_time_per_cycle = chartInterval.ExpectedTimePerCycle
		var suffix = chartInterval.Suffix

		for _, element := range pools["pools"].Arr[:] {
			var daemon_rpc = element.Obj["daemonrpc"].Str
			if localhost_for_daemon_rpc == 1 {
				r, err := regexp.Compile(`.+:`)
				check(err)
				daemon_rpc = r.ReplaceAllString(daemon_rpc, "http://localhost:")
			}

			// Get height
			resp, err := http.Get(element.Obj["daemonrpc"].Str + "getheight")
			check(err)
			defer resp.Body.Close()
			body, _ := ioutil.ReadAll(resp.Body)

			var ch CoinHeight
			err = json.Unmarshal(body, &ch)
			check(err)

			// Get settings
			var jsonStr = []byte(`{"method":"f_get_blockchain_settings"}`)
			req, err := http.NewRequest("POST", daemon_rpc+"json_rpc", bytes.NewBuffer(jsonStr))
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{}
			resp, err = client.Do(req)
			check(err)
			defer resp.Body.Close()
			body, _ = ioutil.ReadAll(resp.Body)

			var cc CoinConfig
			err = json.Unmarshal(body, &cc)
			check(err)

			blocks_per_cycle := int(float64(expected_time_per_cycle) / float64(cc.Result.Core.DIFFICULTYTARGET))

			// Get data
			var transactions_count uint64 = 0
			var transactions_outputs uint64 = 0
			var transactions_fees uint64 = 0
			var transactions_size_culm uint64 = 0
			var transactions_fusion_count uint64 = 0
			var block_current_txs_median_max uint64 = 0
			var blocks_size_culm uint64 = 0
			var blocks_with_penalty uint64 = 0
			charts_dir := dir + "/charts_output/" + strings.ToLower(string(cc.Result.Core.CRYPTONOTENAME))
			start_height, err := strconv.Atoi(string(readFile(charts_dir + "/height" + suffix)))
			if err != nil {
				start_height = 0
			}
			var difficulty_culm uint64 = 0
			var last_timestamp = 0

			if ch.Height-start_height+1 < blocks_per_cycle {
				continue
			}

			// Read hashrate
			hashrate_filename := charts_dir + "/hashrate" + suffix + ".json"
			hashrate_chart := readChart(hashrate_filename)

			// Read transactions count
			transactions_count_filename := charts_dir + "/transactions_count" + suffix + ".json"
			transactions_count_chart := readChart(transactions_count_filename)

			// Read transactions outputs
			transactions_outputs_filename := charts_dir + "/transactions_outputs" + suffix + ".json"
			transactions_outputs_chart := readChart(transactions_outputs_filename)

			// Read transactions fees
			transactions_fees_filename := charts_dir + "/transactions_fees" + suffix + ".json"
			transactions_fees_chart := readChart(transactions_fees_filename)

			// Read transactions size
			transactions_size_avg_filename := charts_dir + "/transactions_size_avg" + suffix + ".json"
			transactions_size_chart := readChart(transactions_size_avg_filename)

			// Read transactions fusion count
			transactions_fusion_count_filename := charts_dir + "/transactions_fusion_count" + suffix + ".json"
			transactions_fusion_count_chart := readChart(transactions_fusion_count_filename)

			// Read block reward
			block_reward_filename := charts_dir + "/block_reward" + suffix + ".json"
			block_reward_chart := readChart(block_reward_filename)

			// Read block current txs median (max)
			block_current_txs_median_max_filename := charts_dir + "/block_current_txs_median_max" + suffix + ".json"
			block_current_txs_median_max_chart := readChart(block_current_txs_median_max_filename)

			// Read blocks size
			blocks_size_filename := charts_dir + "/blocks_size_avg" + suffix + ".json"
			blocks_size_chart := readChart(blocks_size_filename)

			// Read blocks size
			blocks_time_filename := charts_dir + "/blocks_time_avg" + suffix + ".json"
			blocks_time_chart := readChart(blocks_time_filename)

			// Read blocks penalty percentage
			blocks_penalty_percentage_filename := charts_dir + "/blocks_penalty_percentage" + suffix + ".json"
			blocks_penalty_percentage_chart := readChart(blocks_penalty_percentage_filename)

			// Read generated coins
			generated_coins_filename := charts_dir + "/generated_coins" + suffix + ".json"
			generated_coins_chart := readChart(generated_coins_filename)

			// Read difficulty
			difficulty_filename := charts_dir + "/difficulty_avg" + suffix + ".json"
			difficulty_chart := readChart(difficulty_filename)

			for i := start_height + 1; i < ch.Height; i++ {
				var height = i
				height_string := strconv.Itoa(height)
				jsonStr = []byte(`{"method": "f_block_json","params": {"hash": "` + height_string + `"}}`)
				req, err = http.NewRequest("POST", daemon_rpc+"json_rpc", bytes.NewBuffer(jsonStr))
				req.Header.Set("Content-Type", "application/json")

				client = &http.Client{}
				resp, err = client.Do(req)
				check(err)
				defer resp.Body.Close()
				body, _ = ioutil.ReadAll(resp.Body)

				var block Block
				err = json.Unmarshal(body, &block)
				check(err)

				if last_timestamp == 0 {
					last_timestamp = block.Result.Block.Timestamp
				}

				blocks_size_culm += uint64(block.Result.Block.BlockSize)
				block_current_txs_median_max = uint64(math.Max(float64(block_current_txs_median_max), float64(block.Result.Block.SizeMedian)))
				difficulty_culm += uint64(block.Result.Block.Difficulty)
				if block.Result.Block.Penalty != 0 {
					blocks_with_penalty += 1
				}
				for key, element := range block.Result.Block.Transactions {
					if key != 0 {
						if element.Fee != 0 {
							transactions_outputs += uint64(element.AmountOut)
							transactions_fees += uint64(element.Fee)
							transactions_size_culm += uint64(element.Size)
							transactions_count += 1
						} else {
							transactions_fusion_count += 1
						}
					}
				}
				if i%blocks_per_cycle == 1 {
					// Hashrate
					var hashrate uint64 = 0
					var blocks_time_avg uint64 = 0
					var current_time_per_cycle = 0
					if i != 1 {
						if last_timestamp > block.Result.Block.Timestamp {
							// Stupid fix of blockchain timestamp issue
							current_time_per_cycle = block.Result.Block.Timestamp - last_timestamp + 1*60*60
						} else {
							current_time_per_cycle = block.Result.Block.Timestamp - last_timestamp
						}
						last_timestamp = block.Result.Block.Timestamp
					}
					if i != 1 {
						hashrate = uint64(float64(difficulty_culm) / float64(current_time_per_cycle))
					}
					hashrate_chart = addPointToChart(last_timestamp, hashrate, hashrate_chart, hashrate_filename)

					// Transactions count
					transactions_count_chart = addPointToChart(last_timestamp, transactions_count, transactions_count_chart, transactions_count_filename)

					// Transactions outputs
					transactions_outputs_chart = addPointToChart(last_timestamp, transactions_outputs, transactions_outputs_chart, transactions_outputs_filename)

					// Transactions fees
					transactions_fees_chart = addPointToChart(last_timestamp, transactions_fees, transactions_fees_chart, transactions_fees_filename)

					// Transactions size
					var transactions_size_avg uint64 = 0
					if transactions_count != 0 {
						transactions_size_avg = uint64(float64(transactions_size_culm) / float64(transactions_count))
					}
					transactions_size_chart = addPointToChart(last_timestamp, transactions_size_avg, transactions_size_chart, transactions_size_avg_filename)

					// Transactions fusion count
					transactions_fusion_count_chart = addPointToChart(last_timestamp, transactions_fusion_count, transactions_fusion_count_chart, transactions_fusion_count_filename)

					// Block reward
					block_reward := uint64(block.Result.Block.BaseReward)
					block_reward_chart = addPointToChart(last_timestamp, block_reward, block_reward_chart, block_reward_filename)

					// Block current txs median (max)
					block_current_txs_median_max_chart = addPointToChart(last_timestamp, block_current_txs_median_max, block_current_txs_median_max_chart, block_current_txs_median_max_filename)

					// Blocks size (avg)
					blocks_size_avg := uint64(float64(blocks_size_culm) / float64(blocks_per_cycle))
					blocks_size_chart = addPointToChart(last_timestamp, blocks_size_avg, blocks_size_chart, blocks_size_filename)

					// Blocks time (avg)
					if i != 1 {
						blocks_time_avg = uint64(float64(cc.Result.Core.DIFFICULTYTARGET) * (float64(current_time_per_cycle) / float64(expected_time_per_cycle)))
					}
					blocks_time_chart = addPointToChart(last_timestamp, blocks_time_avg, blocks_time_chart, blocks_time_filename)

					// Blocks penalty percenage
					blocks_penalty_percentage := uint64(float64(blocks_with_penalty) * 100 / float64(blocks_per_cycle))
					blocks_penalty_percentage_chart = addPointToChart(last_timestamp, blocks_penalty_percentage, blocks_penalty_percentage_chart, blocks_penalty_percentage_filename)

					// Generated coins
					generated_coins, err := strconv.ParseUint(block.Result.Block.AlreadyGeneratedCoins, 10, 64)
					check(err)
					generated_coins_chart = addPointToChart(last_timestamp, generated_coins, generated_coins_chart, generated_coins_filename)

					// Difficulty
					difficulty := uint64(block.Result.Block.Difficulty)
					difficulty_chart = addPointToChart(last_timestamp, difficulty, difficulty_chart, difficulty_filename)

					// Null aggregated values
					transactions_count = 0
					transactions_outputs = 0
					transactions_fees = 0
					transactions_size_culm = 0
					transactions_fusion_count = 0
					block_current_txs_median_max = 0
					blocks_size_culm = 0
					blocks_with_penalty = 0
					difficulty_culm = 0

					writeFile(charts_dir+"/height"+suffix, []byte(height_string))
					fmt.Println("Network: ", string(cc.Result.Core.CRYPTONOTENAME), "   Height: ", height_string)
				}
			}
		}
	}
}
