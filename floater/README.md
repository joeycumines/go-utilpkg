# floater

Package floater is not the shit in the toilet. Utils for math/big.

## Documentation

Available [here](https://pkg.go.dev/github.com/joeycumines/floater).

## Examples

### FormatDecimalRat

#### Accurate formatting to float

If you want to format a `math/big.Rat` to a decimal string, your stdlib choices
are:

1. `math/big.Rat.FloatString`
2. `math/big.Float.SetRat` -> `math/big.Float.Text`
3. `math/big.Rat.Float64` -> `strconv.FormatFloat`

All three have deficiencies, particularly if you wish for automatic
reasonably-accurate conversion, and especially if your `math/big.Rat` was a
value calculated from floating-point inputs. The `floater.FormatDecimalRat`
implementation supports `strconv.FloatFloat('f', ...)` semantics, including
automatically determining an (approximation of) the number of decimals
necessary. The `floater.FormatDecimalRat` implementation also supports
automatic determination of an appropriate float precision, using the same logic
as `math/big.Float.SetRat`.

See also the
[full example](https://pkg.go.dev/github.com/joeycumines/floater#example-FormatDecimalRat-RoundUpEdgeCase1).

```
prec=-1
ours:  0.049504950495049505
float: 0.04950495049504951
---
prec=16
ours:  0.0495049504950495
float: 0.0495049504950495
rat:   0.0495049504950495
---
prec=17
ours:  0.04950495049504950
float: 0.04950495049504951
rat:   0.04950495049504950
---
prec=18
ours:  0.049504950495049505
float: 0.049504950495049507
rat:   0.049504950495049505
```

## Benchmark results

### FormatDecimalRat

```
goos: darwin
goarch: arm64
pkg: github.com/joeycumines/floater
cpu: Apple M2 Pro
BenchmarkFormatDecimalRat
BenchmarkFormatDecimalRat/DecimalRat_1
BenchmarkFormatDecimalRat/DecimalRat_1/FormatDecimalRat
BenchmarkFormatDecimalRat/DecimalRat_1/FormatDecimalRat-10         	 2168971	       507.6 ns/op	     440 B/op	      20 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_1/big.Rat.FloatString
BenchmarkFormatDecimalRat/DecimalRat_1/big.Rat.FloatString-10      	 3409490	       352.5 ns/op	     320 B/op	      14 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_1/big.Float.Text_specific_prec
BenchmarkFormatDecimalRat/DecimalRat_1/big.Float.Text_specific_prec-10         	 5270211	       217.6 ns/op	     176 B/op	       7 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_1/big.Float.Text_auto_prec
BenchmarkFormatDecimalRat/DecimalRat_1/big.Float.Text_auto_prec-10             	 1575210	       739.9 ns/op	     712 B/op	      20 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_1/strconv.FormatFloat_specific_prec
BenchmarkFormatDecimalRat/DecimalRat_1/strconv.FormatFloat_specific_prec-10    	13318806	        89.65 ns/op	      48 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_1/strconv.FormatFloat_auto_prec
BenchmarkFormatDecimalRat/DecimalRat_1/strconv.FormatFloat_auto_prec-10        	19293753	        62.64 ns/op	      48 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_2
BenchmarkFormatDecimalRat/DecimalRat_2/FormatDecimalRat
BenchmarkFormatDecimalRat/DecimalRat_2/FormatDecimalRat-10                     	 3989487	       304.5 ns/op	     192 B/op	      13 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_2/big.Rat.FloatString
BenchmarkFormatDecimalRat/DecimalRat_2/big.Rat.FloatString-10                  	 6993184	       169.6 ns/op	     144 B/op	       9 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_2/big.Float.Text_specific_prec
BenchmarkFormatDecimalRat/DecimalRat_2/big.Float.Text_specific_prec-10         	10876087	       110.4 ns/op	      48 B/op	       5 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_2/big.Float.Text_auto_prec
BenchmarkFormatDecimalRat/DecimalRat_2/big.Float.Text_auto_prec-10             	 1306072	       939.8 ns/op	     688 B/op	      19 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_2/strconv.FormatFloat_specific_prec
BenchmarkFormatDecimalRat/DecimalRat_2/strconv.FormatFloat_specific_prec-10    	14043032	        88.45 ns/op	      28 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_2/strconv.FormatFloat_auto_prec
BenchmarkFormatDecimalRat/DecimalRat_2/strconv.FormatFloat_auto_prec-10        	17147664	        62.05 ns/op	      28 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_3
BenchmarkFormatDecimalRat/DecimalRat_3/FormatDecimalRat
BenchmarkFormatDecimalRat/DecimalRat_3/FormatDecimalRat-10                     	 2473236	       502.3 ns/op	     408 B/op	      18 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_3/big.Rat.FloatString
BenchmarkFormatDecimalRat/DecimalRat_3/big.Rat.FloatString-10                  	 3310762	       354.2 ns/op	     312 B/op	      14 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_3/big.Float.Text_specific_prec
BenchmarkFormatDecimalRat/DecimalRat_3/big.Float.Text_specific_prec-10         	 5076002	       240.5 ns/op	     160 B/op	       6 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_3/big.Float.Text_auto_prec
BenchmarkFormatDecimalRat/DecimalRat_3/big.Float.Text_auto_prec-10             	 1421373	       843.6 ns/op	     712 B/op	      20 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_3/strconv.FormatFloat_specific_prec
BenchmarkFormatDecimalRat/DecimalRat_3/strconv.FormatFloat_specific_prec-10    	 8490703	       142.3 ns/op	      48 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_3/strconv.FormatFloat_auto_prec
BenchmarkFormatDecimalRat/DecimalRat_3/strconv.FormatFloat_auto_prec-10        	16048832	        76.68 ns/op	      48 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_4
BenchmarkFormatDecimalRat/DecimalRat_4/FormatDecimalRat
BenchmarkFormatDecimalRat/DecimalRat_4/FormatDecimalRat-10                     	 2647880	       450.8 ns/op	     344 B/op	      18 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_4/big.Rat.FloatString
BenchmarkFormatDecimalRat/DecimalRat_4/big.Rat.FloatString-10                  	 3931701	       306.0 ns/op	     288 B/op	      14 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_4/big.Float.Text_specific_prec
BenchmarkFormatDecimalRat/DecimalRat_4/big.Float.Text_specific_prec-10         	 5390155	       224.7 ns/op	     136 B/op	       6 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_4/big.Float.Text_auto_prec
BenchmarkFormatDecimalRat/DecimalRat_4/big.Float.Text_auto_prec-10             	 1412846	       860.9 ns/op	     712 B/op	      20 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_4/strconv.FormatFloat_specific_prec
BenchmarkFormatDecimalRat/DecimalRat_4/strconv.FormatFloat_specific_prec-10    	 9188170	       132.7 ns/op	      40 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_4/strconv.FormatFloat_auto_prec
BenchmarkFormatDecimalRat/DecimalRat_4/strconv.FormatFloat_auto_prec-10        	15505837	        76.23 ns/op	      48 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_5
BenchmarkFormatDecimalRat/DecimalRat_5/FormatDecimalRat
BenchmarkFormatDecimalRat/DecimalRat_5/FormatDecimalRat-10                     	 1692486	       708.8 ns/op	     800 B/op	      19 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_5/big.Rat.FloatString
BenchmarkFormatDecimalRat/DecimalRat_5/big.Rat.FloatString-10                  	 2114258	       568.0 ns/op	     616 B/op	      15 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_5/big.Float.Text_specific_prec
BenchmarkFormatDecimalRat/DecimalRat_5/big.Float.Text_specific_prec-10         	  681764	      1766 ns/op	     960 B/op	       8 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_5/big.Float.Text_auto_prec
BenchmarkFormatDecimalRat/DecimalRat_5/big.Float.Text_auto_prec-10             	  218082	      5400 ns/op	    2856 B/op	      26 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_5/strconv.FormatFloat_specific_prec
BenchmarkFormatDecimalRat/DecimalRat_5/strconv.FormatFloat_specific_prec-10    	 5463564	       226.1 ns/op	     336 B/op	       3 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_5/strconv.FormatFloat_auto_prec
BenchmarkFormatDecimalRat/DecimalRat_5/strconv.FormatFloat_auto_prec-10        	15554005	        76.97 ns/op	      48 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_6
BenchmarkFormatDecimalRat/DecimalRat_6/FormatDecimalRat
BenchmarkFormatDecimalRat/DecimalRat_6/FormatDecimalRat-10                     	 3146209	       382.4 ns/op	     264 B/op	      15 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_6/big.Rat.FloatString
BenchmarkFormatDecimalRat/DecimalRat_6/big.Rat.FloatString-10                  	 5740561	       208.8 ns/op	     200 B/op	      10 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_6/big.Float.Text_specific_prec
BenchmarkFormatDecimalRat/DecimalRat_6/big.Float.Text_specific_prec-10         	 3170988	       375.6 ns/op	     224 B/op	       7 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_6/big.Float.Text_auto_prec
BenchmarkFormatDecimalRat/DecimalRat_6/big.Float.Text_auto_prec-10             	  923235	      1278 ns/op	     960 B/op	      22 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_6/strconv.FormatFloat_specific_prec
BenchmarkFormatDecimalRat/DecimalRat_6/strconv.FormatFloat_specific_prec-10    	 4564881	       264.3 ns/op	      32 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_6/strconv.FormatFloat_auto_prec
BenchmarkFormatDecimalRat/DecimalRat_6/strconv.FormatFloat_auto_prec-10        	18011402	        66.66 ns/op	      32 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_7
BenchmarkFormatDecimalRat/DecimalRat_7/FormatDecimalRat
BenchmarkFormatDecimalRat/DecimalRat_7/FormatDecimalRat-10                     	 2824808	       417.0 ns/op	     272 B/op	      16 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_7/big.Rat.FloatString
BenchmarkFormatDecimalRat/DecimalRat_7/big.Rat.FloatString-10                  	 4565226	       263.6 ns/op	     264 B/op	      14 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_7/big.Float.Text_specific_prec
BenchmarkFormatDecimalRat/DecimalRat_7/big.Float.Text_specific_prec-10         	 3281704	       360.9 ns/op	     224 B/op	       7 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_7/big.Float.Text_auto_prec
BenchmarkFormatDecimalRat/DecimalRat_7/big.Float.Text_auto_prec-10             	  955771	      1247 ns/op	     960 B/op	      22 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_7/strconv.FormatFloat_specific_prec
BenchmarkFormatDecimalRat/DecimalRat_7/strconv.FormatFloat_specific_prec-10    	 8037579	       149.8 ns/op	      28 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/DecimalRat_7/strconv.FormatFloat_auto_prec
BenchmarkFormatDecimalRat/DecimalRat_7/strconv.FormatFloat_auto_prec-10        	17551749	        68.78 ns/op	      32 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/SmallInteger
BenchmarkFormatDecimalRat/SmallInteger/FormatDecimalRat
BenchmarkFormatDecimalRat/SmallInteger/FormatDecimalRat-10                     	19018197	        64.09 ns/op	      16 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/SmallInteger/big.Rat.FloatString
BenchmarkFormatDecimalRat/SmallInteger/big.Rat.FloatString-10                  	17237622	        70.84 ns/op	      16 B/op	       3 allocs/op
BenchmarkFormatDecimalRat/SmallInteger/big.Float.Text_specific_prec
BenchmarkFormatDecimalRat/SmallInteger/big.Float.Text_specific_prec-10         	11231652	       105.7 ns/op	      48 B/op	       5 allocs/op
BenchmarkFormatDecimalRat/SmallInteger/big.Float.Text_auto_prec
BenchmarkFormatDecimalRat/SmallInteger/big.Float.Text_auto_prec-10             	 1625382	       736.7 ns/op	     784 B/op	      20 allocs/op
BenchmarkFormatDecimalRat/SmallInteger/strconv.FormatFloat_specific_prec
BenchmarkFormatDecimalRat/SmallInteger/strconv.FormatFloat_specific_prec-10    	14435098	        83.51 ns/op	      26 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/SmallInteger/strconv.FormatFloat_auto_prec
BenchmarkFormatDecimalRat/SmallInteger/strconv.FormatFloat_auto_prec-10        	21444238	        55.81 ns/op	      26 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/LargeInteger
BenchmarkFormatDecimalRat/LargeInteger/FormatDecimalRat
BenchmarkFormatDecimalRat/LargeInteger/FormatDecimalRat-10                     	 8930154	       132.8 ns/op	     112 B/op	       3 allocs/op
BenchmarkFormatDecimalRat/LargeInteger/big.Rat.FloatString
BenchmarkFormatDecimalRat/LargeInteger/big.Rat.FloatString-10                  	 8137422	       148.1 ns/op	     144 B/op	       4 allocs/op
BenchmarkFormatDecimalRat/LargeInteger/big.Float.Text_specific_prec
BenchmarkFormatDecimalRat/LargeInteger/big.Float.Text_specific_prec-10         	 5764550	       208.4 ns/op	     240 B/op	       7 allocs/op
BenchmarkFormatDecimalRat/LargeInteger/big.Float.Text_auto_prec
BenchmarkFormatDecimalRat/LargeInteger/big.Float.Text_auto_prec-10             	 1559626	       775.9 ns/op	     720 B/op	      18 allocs/op
BenchmarkFormatDecimalRat/LargeInteger/strconv.FormatFloat_specific_prec
BenchmarkFormatDecimalRat/LargeInteger/strconv.FormatFloat_specific_prec-10    	 9095272	       131.3 ns/op	     104 B/op	       3 allocs/op
BenchmarkFormatDecimalRat/LargeInteger/strconv.FormatFloat_auto_prec
BenchmarkFormatDecimalRat/LargeInteger/strconv.FormatFloat_auto_prec-10        	12377311	        99.88 ns/op	     104 B/op	       3 allocs/op
BenchmarkFormatDecimalRat/SmallRationalAutoPrec
BenchmarkFormatDecimalRat/SmallRationalAutoPrec/FormatDecimalRat
BenchmarkFormatDecimalRat/SmallRationalAutoPrec/FormatDecimalRat-10            	 2534362	       471.7 ns/op	     464 B/op	      18 allocs/op
BenchmarkFormatDecimalRat/SmallRationalAutoPrec/big.Rat.FloatString
BenchmarkFormatDecimalRat/SmallRationalAutoPrec/big.Rat.FloatString-10         	 3269518	       362.3 ns/op	     376 B/op	      14 allocs/op
BenchmarkFormatDecimalRat/SmallRationalAutoPrec/big.Float.Text_specific_prec
BenchmarkFormatDecimalRat/SmallRationalAutoPrec/big.Float.Text_specific_prec-10         	 2912564	       415.0 ns/op	     256 B/op	       7 allocs/op
BenchmarkFormatDecimalRat/SmallRationalAutoPrec/big.Float.Text_auto_prec
BenchmarkFormatDecimalRat/SmallRationalAutoPrec/big.Float.Text_auto_prec-10             	  847768	      1352 ns/op	    1000 B/op	      23 allocs/op
BenchmarkFormatDecimalRat/SmallRationalAutoPrec/strconv.FormatFloat_specific_prec
BenchmarkFormatDecimalRat/SmallRationalAutoPrec/strconv.FormatFloat_specific_prec-10    	 6557478	       179.8 ns/op	      56 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/SmallRationalAutoPrec/strconv.FormatFloat_auto_prec
BenchmarkFormatDecimalRat/SmallRationalAutoPrec/strconv.FormatFloat_auto_prec-10        	14990606	        80.60 ns/op	      48 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/LargeRationalSpecificPrec
BenchmarkFormatDecimalRat/LargeRationalSpecificPrec/FormatDecimalRat
BenchmarkFormatDecimalRat/LargeRationalSpecificPrec/FormatDecimalRat-10                 	14185249	        84.87 ns/op	      32 B/op	       3 allocs/op
BenchmarkFormatDecimalRat/LargeRationalSpecificPrec/big.Rat.FloatString
BenchmarkFormatDecimalRat/LargeRationalSpecificPrec/big.Rat.FloatString-10              	12275151	        97.14 ns/op	      48 B/op	       4 allocs/op
BenchmarkFormatDecimalRat/LargeRationalSpecificPrec/big.Float.Text_specific_prec
BenchmarkFormatDecimalRat/LargeRationalSpecificPrec/big.Float.Text_specific_prec-10     	 9717205	       121.2 ns/op	      64 B/op	       5 allocs/op
BenchmarkFormatDecimalRat/LargeRationalSpecificPrec/big.Float.Text_auto_prec
BenchmarkFormatDecimalRat/LargeRationalSpecificPrec/big.Float.Text_auto_prec-10         	  645265	      1819 ns/op	    1264 B/op	      20 allocs/op
BenchmarkFormatDecimalRat/LargeRationalSpecificPrec/strconv.FormatFloat_specific_prec
BenchmarkFormatDecimalRat/LargeRationalSpecificPrec/strconv.FormatFloat_specific_prec-10         	12384586	        95.92 ns/op	      40 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/LargeRationalSpecificPrec/strconv.FormatFloat_auto_prec
BenchmarkFormatDecimalRat/LargeRationalSpecificPrec/strconv.FormatFloat_auto_prec-10             	20421230	        57.75 ns/op	      26 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/HighPrecision
BenchmarkFormatDecimalRat/HighPrecision/FormatDecimalRat
BenchmarkFormatDecimalRat/HighPrecision/FormatDecimalRat-10                                      	 1489424	       802.9 ns/op	     960 B/op	      20 allocs/op
BenchmarkFormatDecimalRat/HighPrecision/big.Rat.FloatString
BenchmarkFormatDecimalRat/HighPrecision/big.Rat.FloatString-10                                   	 1881495	       642.6 ns/op	     704 B/op	      14 allocs/op
BenchmarkFormatDecimalRat/HighPrecision/big.Float.Text_specific_prec
BenchmarkFormatDecimalRat/HighPrecision/big.Float.Text_specific_prec-10                          	 2457735	       484.3 ns/op	     424 B/op	       7 allocs/op
BenchmarkFormatDecimalRat/HighPrecision/big.Float.Text_auto_prec
BenchmarkFormatDecimalRat/HighPrecision/big.Float.Text_auto_prec-10                              	  881433	      1370 ns/op	    1000 B/op	      23 allocs/op
BenchmarkFormatDecimalRat/HighPrecision/strconv.FormatFloat_specific_prec
BenchmarkFormatDecimalRat/HighPrecision/strconv.FormatFloat_specific_prec-10                     	 4674649	       261.4 ns/op	     224 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/HighPrecision/strconv.FormatFloat_auto_prec
BenchmarkFormatDecimalRat/HighPrecision/strconv.FormatFloat_auto_prec-10                         	14974130	        80.28 ns/op	      48 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/EdgeZero
BenchmarkFormatDecimalRat/EdgeZero/FormatDecimalRat
BenchmarkFormatDecimalRat/EdgeZero/FormatDecimalRat-10                                           	37886012	        31.50 ns/op	      16 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/EdgeZero/big.Rat.FloatString
BenchmarkFormatDecimalRat/EdgeZero/big.Rat.FloatString-10                                        	28659710	        41.57 ns/op	      16 B/op	       3 allocs/op
BenchmarkFormatDecimalRat/EdgeZero/big.Float.Text_specific_prec
BenchmarkFormatDecimalRat/EdgeZero/big.Float.Text_specific_prec-10                               	35580681	        33.39 ns/op	      24 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/EdgeZero/big.Float.Text_auto_prec
BenchmarkFormatDecimalRat/EdgeZero/big.Float.Text_auto_prec-10                                   	60768210	        19.10 ns/op	      16 B/op	       1 allocs/op
BenchmarkFormatDecimalRat/EdgeZero/strconv.FormatFloat_specific_prec
BenchmarkFormatDecimalRat/EdgeZero/strconv.FormatFloat_specific_prec-10                          	24601317	        48.44 ns/op	      32 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/EdgeZero/strconv.FormatFloat_auto_prec
BenchmarkFormatDecimalRat/EdgeZero/strconv.FormatFloat_auto_prec-10                              	50813724	        23.17 ns/op	      24 B/op	       1 allocs/op
BenchmarkFormatDecimalRat/LargeFloatPrec
BenchmarkFormatDecimalRat/LargeFloatPrec/FormatDecimalRat
BenchmarkFormatDecimalRat/LargeFloatPrec/FormatDecimalRat-10                                     	 1477929	       788.7 ns/op	     888 B/op	      22 allocs/op
BenchmarkFormatDecimalRat/LargeFloatPrec/big.Rat.FloatString
BenchmarkFormatDecimalRat/LargeFloatPrec/big.Rat.FloatString-10                                  	 1964488	       604.7 ns/op	     688 B/op	      16 allocs/op
BenchmarkFormatDecimalRat/LargeFloatPrec/big.Float.Text_specific_prec
BenchmarkFormatDecimalRat/LargeFloatPrec/big.Float.Text_specific_prec-10                         	  651255	      1793 ns/op	    1104 B/op	       9 allocs/op
BenchmarkFormatDecimalRat/LargeFloatPrec/big.Float.Text_auto_prec
BenchmarkFormatDecimalRat/LargeFloatPrec/big.Float.Text_auto_prec-10                             	  223387	      5341 ns/op	    2856 B/op	      26 allocs/op
BenchmarkFormatDecimalRat/LargeFloatPrec/strconv.FormatFloat_specific_prec
BenchmarkFormatDecimalRat/LargeFloatPrec/strconv.FormatFloat_specific_prec-10                    	 6757008	       175.4 ns/op	     336 B/op	       3 allocs/op
BenchmarkFormatDecimalRat/LargeFloatPrec/strconv.FormatFloat_auto_prec
BenchmarkFormatDecimalRat/LargeFloatPrec/strconv.FormatFloat_auto_prec-10                        	21907554	        54.68 ns/op	      40 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/ExtremelyLargeInteger
BenchmarkFormatDecimalRat/ExtremelyLargeInteger/FormatDecimalRat
BenchmarkFormatDecimalRat/ExtremelyLargeInteger/FormatDecimalRat-10                              	 3649136	       330.1 ns/op	     304 B/op	       3 allocs/op
BenchmarkFormatDecimalRat/ExtremelyLargeInteger/big.Rat.FloatString
BenchmarkFormatDecimalRat/ExtremelyLargeInteger/big.Rat.FloatString-10                           	 3390589	       354.1 ns/op	     416 B/op	       4 allocs/op
BenchmarkFormatDecimalRat/ExtremelyLargeInteger/big.Float.Text_specific_prec
BenchmarkFormatDecimalRat/ExtremelyLargeInteger/big.Float.Text_specific_prec-10                  	 2887112	       415.9 ns/op	     624 B/op	       7 allocs/op
BenchmarkFormatDecimalRat/ExtremelyLargeInteger/big.Float.Text_auto_prec
BenchmarkFormatDecimalRat/ExtremelyLargeInteger/big.Float.Text_auto_prec-10                      	  624890	      1849 ns/op	    1648 B/op	      18 allocs/op
BenchmarkFormatDecimalRat/ExtremelyLargeInteger/strconv.FormatFloat_specific_prec
BenchmarkFormatDecimalRat/ExtremelyLargeInteger/strconv.FormatFloat_specific_prec-10             	 1897298	       628.3 ns/op	     248 B/op	       3 allocs/op
BenchmarkFormatDecimalRat/ExtremelyLargeInteger/strconv.FormatFloat_auto_prec
BenchmarkFormatDecimalRat/ExtremelyLargeInteger/strconv.FormatFloat_auto_prec-10                 	 6313666	       186.7 ns/op	     472 B/op	       5 allocs/op
BenchmarkFormatDecimalRat/LargeRationalHighPrecision
BenchmarkFormatDecimalRat/LargeRationalHighPrecision/FormatDecimalRat
BenchmarkFormatDecimalRat/LargeRationalHighPrecision/FormatDecimalRat-10                         	 1289052	       924.5 ns/op	    1056 B/op	      21 allocs/op
BenchmarkFormatDecimalRat/LargeRationalHighPrecision/big.Rat.FloatString
BenchmarkFormatDecimalRat/LargeRationalHighPrecision/big.Rat.FloatString-10                      	 1640667	       722.9 ns/op	     800 B/op	      15 allocs/op
BenchmarkFormatDecimalRat/LargeRationalHighPrecision/big.Float.Text_specific_prec
BenchmarkFormatDecimalRat/LargeRationalHighPrecision/big.Float.Text_specific_prec-10             	 3058802	       389.0 ns/op	     656 B/op	       8 allocs/op
BenchmarkFormatDecimalRat/LargeRationalHighPrecision/big.Float.Text_auto_prec
BenchmarkFormatDecimalRat/LargeRationalHighPrecision/big.Float.Text_auto_prec-10                 	 1363954	       870.2 ns/op	     800 B/op	      21 allocs/op
BenchmarkFormatDecimalRat/LargeRationalHighPrecision/strconv.FormatFloat_specific_prec
BenchmarkFormatDecimalRat/LargeRationalHighPrecision/strconv.FormatFloat_specific_prec-10        	 5568057	       219.4 ns/op	     448 B/op	       3 allocs/op
BenchmarkFormatDecimalRat/LargeRationalHighPrecision/strconv.FormatFloat_auto_prec
BenchmarkFormatDecimalRat/LargeRationalHighPrecision/strconv.FormatFloat_auto_prec-10            	16221906	        74.54 ns/op	      48 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/SmallRationalExtremePrecision
BenchmarkFormatDecimalRat/SmallRationalExtremePrecision/FormatDecimalRat
BenchmarkFormatDecimalRat/SmallRationalExtremePrecision/FormatDecimalRat-10                      	  165405	      7163 ns/op	    8349 B/op	      29 allocs/op
BenchmarkFormatDecimalRat/SmallRationalExtremePrecision/big.Rat.FloatString
BenchmarkFormatDecimalRat/SmallRationalExtremePrecision/big.Rat.FloatString-10                   	  182168	      6575 ns/op	    6372 B/op	      23 allocs/op
BenchmarkFormatDecimalRat/SmallRationalExtremePrecision/big.Float.Text_specific_prec
BenchmarkFormatDecimalRat/SmallRationalExtremePrecision/big.Float.Text_specific_prec-10          	  954640	      1218 ns/op	    2248 B/op	       7 allocs/op
BenchmarkFormatDecimalRat/SmallRationalExtremePrecision/big.Float.Text_auto_prec
BenchmarkFormatDecimalRat/SmallRationalExtremePrecision/big.Float.Text_auto_prec-10              	  848620	      1361 ns/op	    1000 B/op	      23 allocs/op
BenchmarkFormatDecimalRat/SmallRationalExtremePrecision/strconv.FormatFloat_specific_prec
BenchmarkFormatDecimalRat/SmallRationalExtremePrecision/strconv.FormatFloat_specific_prec-10     	 1237326	       971.2 ns/op	    2048 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/SmallRationalExtremePrecision/strconv.FormatFloat_auto_prec
BenchmarkFormatDecimalRat/SmallRationalExtremePrecision/strconv.FormatFloat_auto_prec-10         	14577657	        80.76 ns/op	      48 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/LargeRationalLargeFloatPrec
BenchmarkFormatDecimalRat/LargeRationalLargeFloatPrec/FormatDecimalRat
BenchmarkFormatDecimalRat/LargeRationalLargeFloatPrec/FormatDecimalRat-10                        	  545242	      2147 ns/op	    2865 B/op	      27 allocs/op
BenchmarkFormatDecimalRat/LargeRationalLargeFloatPrec/big.Rat.FloatString
BenchmarkFormatDecimalRat/LargeRationalLargeFloatPrec/big.Rat.FloatString-10                     	  638080	      1901 ns/op	    2105 B/op	      21 allocs/op
BenchmarkFormatDecimalRat/LargeRationalLargeFloatPrec/big.Float.Text_specific_prec
BenchmarkFormatDecimalRat/LargeRationalLargeFloatPrec/big.Float.Text_specific_prec-10            	   50710	     23560 ns/op	    3441 B/op	       9 allocs/op
BenchmarkFormatDecimalRat/LargeRationalLargeFloatPrec/big.Float.Text_auto_prec
BenchmarkFormatDecimalRat/LargeRationalLargeFloatPrec/big.Float.Text_auto_prec-10                	   15801	     74854 ns/op	   10061 B/op	      31 allocs/op
BenchmarkFormatDecimalRat/LargeRationalLargeFloatPrec/strconv.FormatFloat_specific_prec
BenchmarkFormatDecimalRat/LargeRationalLargeFloatPrec/strconv.FormatFloat_specific_prec-10       	 3546964	       334.3 ns/op	     640 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/LargeRationalLargeFloatPrec/strconv.FormatFloat_auto_prec
BenchmarkFormatDecimalRat/LargeRationalLargeFloatPrec/strconv.FormatFloat_auto_prec-10           	20512221	        58.41 ns/op	      26 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/LargeRationalExtremelyLargeFloatPrec
BenchmarkFormatDecimalRat/LargeRationalExtremelyLargeFloatPrec/FormatDecimalRat
BenchmarkFormatDecimalRat/LargeRationalExtremelyLargeFloatPrec/FormatDecimalRat-10               	     471	   2552662 ns/op	  776811 B/op	     321 allocs/op
BenchmarkFormatDecimalRat/LargeRationalExtremelyLargeFloatPrec/big.Rat.FloatString
BenchmarkFormatDecimalRat/LargeRationalExtremelyLargeFloatPrec/big.Rat.FloatString-10            	     471	   2545415 ns/op	  600754 B/op	     311 allocs/op
BenchmarkFormatDecimalRat/LargeRationalExtremelyLargeFloatPrec/big.Float.Text_specific_prec
BenchmarkFormatDecimalRat/LargeRationalExtremelyLargeFloatPrec/big.Float.Text_specific_prec-10   	       2	 901303584 ns/op	 1300160 B/op	     296 allocs/op
BenchmarkFormatDecimalRat/LargeRationalExtremelyLargeFloatPrec/big.Float.Text_auto_prec
BenchmarkFormatDecimalRat/LargeRationalExtremelyLargeFloatPrec/big.Float.Text_auto_prec-10       	       1	2698766125 ns/op	 3901464 B/op	     903 allocs/op
BenchmarkFormatDecimalRat/LargeRationalExtremelyLargeFloatPrec/strconv.FormatFloat_specific_prec
BenchmarkFormatDecimalRat/LargeRationalExtremelyLargeFloatPrec/strconv.FormatFloat_specific_prec-10         	   24925	     47629 ns/op	  131072 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/LargeRationalExtremelyLargeFloatPrec/strconv.FormatFloat_auto_prec
BenchmarkFormatDecimalRat/LargeRationalExtremelyLargeFloatPrec/strconv.FormatFloat_auto_prec-10             	20095803	        59.01 ns/op	      26 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/NearZeroHighPrecision
BenchmarkFormatDecimalRat/NearZeroHighPrecision/FormatDecimalRat
BenchmarkFormatDecimalRat/NearZeroHighPrecision/FormatDecimalRat-10                                         	 1391803	       861.0 ns/op	     928 B/op	      25 allocs/op
BenchmarkFormatDecimalRat/NearZeroHighPrecision/big.Rat.FloatString
BenchmarkFormatDecimalRat/NearZeroHighPrecision/big.Rat.FloatString-10                                      	 2916322	       400.2 ns/op	     432 B/op	      14 allocs/op
BenchmarkFormatDecimalRat/NearZeroHighPrecision/big.Float.Text_specific_prec
BenchmarkFormatDecimalRat/NearZeroHighPrecision/big.Float.Text_specific_prec-10                             	  962440	      1245 ns/op	     576 B/op	       8 allocs/op
BenchmarkFormatDecimalRat/NearZeroHighPrecision/big.Float.Text_auto_prec
BenchmarkFormatDecimalRat/NearZeroHighPrecision/big.Float.Text_auto_prec-10                                 	  318824	      3804 ns/op	    1736 B/op	      25 allocs/op
BenchmarkFormatDecimalRat/NearZeroHighPrecision/strconv.FormatFloat_specific_prec
BenchmarkFormatDecimalRat/NearZeroHighPrecision/strconv.FormatFloat_specific_prec-10                        	 2067316	       584.5 ns/op	      96 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/NearZeroHighPrecision/strconv.FormatFloat_auto_prec
BenchmarkFormatDecimalRat/NearZeroHighPrecision/strconv.FormatFloat_auto_prec-10                            	10607116	       111.7 ns/op	     120 B/op	       3 allocs/op
BenchmarkFormatDecimalRat/RepeatingDecimalHighPrecision
BenchmarkFormatDecimalRat/RepeatingDecimalHighPrecision/FormatDecimalRat
BenchmarkFormatDecimalRat/RepeatingDecimalHighPrecision/FormatDecimalRat-10                                 	  343083	      3463 ns/op	    4274 B/op	      27 allocs/op
BenchmarkFormatDecimalRat/RepeatingDecimalHighPrecision/big.Rat.FloatString
BenchmarkFormatDecimalRat/RepeatingDecimalHighPrecision/big.Rat.FloatString-10                              	  380966	      3084 ns/op	    3241 B/op	      21 allocs/op
BenchmarkFormatDecimalRat/RepeatingDecimalHighPrecision/big.Float.Text_specific_prec
BenchmarkFormatDecimalRat/RepeatingDecimalHighPrecision/big.Float.Text_specific_prec-10                     	 1478578	       807.4 ns/op	    1224 B/op	       7 allocs/op
BenchmarkFormatDecimalRat/RepeatingDecimalHighPrecision/big.Float.Text_auto_prec
BenchmarkFormatDecimalRat/RepeatingDecimalHighPrecision/big.Float.Text_auto_prec-10                         	  863964	      1381 ns/op	    1000 B/op	      23 allocs/op
BenchmarkFormatDecimalRat/RepeatingDecimalHighPrecision/strconv.FormatFloat_specific_prec
BenchmarkFormatDecimalRat/RepeatingDecimalHighPrecision/strconv.FormatFloat_specific_prec-10                	 2043848	       570.9 ns/op	    1024 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/RepeatingDecimalHighPrecision/strconv.FormatFloat_auto_prec
BenchmarkFormatDecimalRat/RepeatingDecimalHighPrecision/strconv.FormatFloat_auto_prec-10                    	14437392	        81.47 ns/op	      48 B/op	       2 allocs/op
BenchmarkFormatDecimalRat/VeryUnevenLarge
BenchmarkFormatDecimalRat/VeryUnevenLarge/FormatDecimalRat
BenchmarkFormatDecimalRat/VeryUnevenLarge/FormatDecimalRat-10                                               	 1593127	       756.0 ns/op	     848 B/op	      21 allocs/op
BenchmarkFormatDecimalRat/VeryUnevenLarge/big.Rat.FloatString
BenchmarkFormatDecimalRat/VeryUnevenLarge/big.Rat.FloatString-10                                            	 2024514	       591.4 ns/op	     720 B/op	      16 allocs/op
BenchmarkFormatDecimalRat/VeryUnevenLarge/big.Float.Text_specific_prec
BenchmarkFormatDecimalRat/VeryUnevenLarge/big.Float.Text_specific_prec-10                                   	 3008886	       397.1 ns/op	     576 B/op	       8 allocs/op
BenchmarkFormatDecimalRat/VeryUnevenLarge/big.Float.Text_auto_prec
BenchmarkFormatDecimalRat/VeryUnevenLarge/big.Float.Text_auto_prec-10                                       	  990708	      1162 ns/op	    1184 B/op	      22 allocs/op
BenchmarkFormatDecimalRat/VeryUnevenLarge/strconv.FormatFloat_specific_prec
BenchmarkFormatDecimalRat/VeryUnevenLarge/strconv.FormatFloat_specific_prec-10                              	 6518438	       184.2 ns/op	     272 B/op	       3 allocs/op
BenchmarkFormatDecimalRat/VeryUnevenLarge/strconv.FormatFloat_auto_prec
BenchmarkFormatDecimalRat/VeryUnevenLarge/strconv.FormatFloat_auto_prec-10                                  	11787958	       101.5 ns/op	     104 B/op	       3 allocs/op
PASS
```
